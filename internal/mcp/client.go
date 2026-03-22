// MCP 客户端模块
// 实现 MCP (Model Context Protocol) 的 Streamable HTTP 传输层。
// 通过 JSON-RPC 2.0 协议调用 EnergyPlus-Agent-try2 的 Python MCP Server。
// 支持 Initialize 握手（获取 Session ID）和 tools/call 工具调用。
// 所有调用均记录详细日志，包括请求参数和响应结果。

package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// rpcRequest JSON-RPC 2.0 请求
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// rpcResponse JSON-RPC 2.0 响应
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// rpcError JSON-RPC 错误
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolCallResult tools/call 的响应结果
type toolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// initParams initialize 方法的参数
type initParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

// Client MCP HTTP 客户端
type Client struct {
	baseURL    string
	sessionID  string
	httpClient *http.Client
	idCounter  atomic.Int64
}

// NewClient 创建 MCP 客户端
func NewClient(baseURL string, timeoutSec int) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
			Transport: &http.Transport{
				Proxy: nil, // 禁用系统代理，直连本地 MCP server
			},
		},
	}
}

// Initialize 与 MCP Server 握手，获取 Session ID
// 必须在调用工具前执行
func (c *Client) Initialize(ctx context.Context) error {
	slog.Debug("[MCP] 开始初始化握手", "base_url", c.baseURL)

	params := initParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    map[string]any{},
	}
	params.ClientInfo.Name = "energyplus-agent"
	params.ClientInfo.Version = "0.1.0"

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      c.idCounter.Add(1),
		Method:  "initialize",
		Params:  params,
	}

	resp, headers, err := c.doRPC(ctx, req)
	if err != nil {
		return fmt.Errorf("MCP 初始化失败: %w", err)
	}

	// 保存 Session ID（部分 MCP Server 通过 header 返回）
	if sid := headers.Get("Mcp-Session-Id"); sid != "" {
		c.sessionID = sid
		slog.Debug("[MCP] 获取到 Session ID", "session_id", sid)
	}

	// 发送 notifications/initialized 确认
	notif := rpcRequest{
		JSONRPC: "2.0",
		ID:      c.idCounter.Add(1),
		Method:  "notifications/initialized",
		Params:  map[string]any{},
	}
	_, _, _ = c.doRPC(ctx, notif) // 忽略响应

	slog.Debug("[MCP] 初始化完成", "result_len", len(resp.Result))
	return nil
}

// CallTool 调用 MCP 工具
// name: 工具名称（如 "workflow_load_yaml"）
// args: 工具参数
// 返回工具输出的文本内容
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	slog.Debug("[MCP→] 调用工具", "tool", name, "args", args)

	if args == nil {
		args = map[string]any{}
	}

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      c.idCounter.Add(1),
		Method:  "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": args,
		},
	}

	resp, _, err := c.doRPC(ctx, req)
	if err != nil {
		return "", fmt.Errorf("调用 MCP 工具 [%s] 失败: %w", name, err)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("MCP 工具 [%s] 返回错误 (code=%d): %s",
			name, resp.Error.Code, resp.Error.Message)
	}

	// 解析工具调用结果
	var result toolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		// 兼容直接返回字符串的情况
		var rawStr string
		if json.Unmarshal(resp.Result, &rawStr) == nil {
			slog.Debug("[MCP←] 工具返回（原始字符串）", "tool", name, "len", len(rawStr))
			return rawStr, nil
		}
		return "", fmt.Errorf("解析 MCP 工具结果失败 [%s]: %w", name, err)
	}

	if result.IsError {
		// 提取错误文本
		text := ""
		for _, c := range result.Content {
			if c.Type == "text" {
				text += c.Text
			}
		}
		return "", fmt.Errorf("MCP 工具 [%s] 执行失败: %s", name, text)
	}

	// 拼接所有 text 类型内容
	var sb strings.Builder
	for _, c := range result.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	output := sb.String()

	slog.Debug("[MCP←] 工具返回成功", "tool", name, "result_len", len(output))
	return output, nil
}

// doRPC 执行 JSON-RPC 请求，返回响应和响应头
func (c *Client) doRPC(ctx context.Context, req rpcRequest) (*rpcResponse, http.Header, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("序列化 RPC 请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/mcp/", bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("MCP HTTP 请求失败: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusAccepted {
		// 202: 服务器接受了请求但没有响应体（通知类消息）
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID}, httpResp.Header, nil
	}

	if httpResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(httpResp.Body)
		return nil, httpResp.Header, fmt.Errorf(
			"MCP Server 返回非 200 状态码 %d: %s",
			httpResp.StatusCode, string(bodyBytes))
	}

	contentType := httpResp.Header.Get("Content-Type")
	var rpcResp rpcResponse

	if strings.Contains(contentType, "text/event-stream") {
		// SSE 格式：读取第一个 data 行
		scanner := bufio.NewScanner(httpResp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				if data == "[DONE]" {
					break
				}
				if err := json.Unmarshal([]byte(data), &rpcResp); err != nil {
					return nil, httpResp.Header, fmt.Errorf("解析 SSE 响应失败: %w", err)
				}
				break
			}
		}
	} else {
		// 普通 JSON 响应
		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return nil, httpResp.Header, fmt.Errorf("读取响应体失败: %w", err)
		}
		if err := json.Unmarshal(respBody, &rpcResp); err != nil {
			return nil, httpResp.Header, fmt.Errorf("解析 JSON-RPC 响应失败: %w\n原始: %s", err, string(respBody))
		}
	}

	return &rpcResp, httpResp.Header, nil
}
