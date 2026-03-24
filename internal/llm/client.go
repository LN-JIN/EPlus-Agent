// LLM 客户端模块
// 封装对 OpenAI 兼容接口的 HTTP 调用，支持流式（SSE）和非流式两种模式。
// 流式模式下通过 onToken 回调实时将 token 推送给调用方展示。
// 所有请求/响应均通过日志模块完整记录，方便追踪 LLM 的推理过程。

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client OpenAI 兼容 LLM 客户端
type Client struct {
	BaseURL    string
	APIKey     string
	Model      string
	Timeout    time.Duration
	Temperature float64
	httpClient *http.Client
}

// NewClient 创建 LLM 客户端
func NewClient(baseURL, apiKey, model string, timeoutSec int, temperature float64) *Client {
	return &Client{
		BaseURL:     baseURL,
		APIKey:      apiKey,
		Model:       model,
		Timeout:     time.Duration(timeoutSec) * time.Second,
		Temperature: temperature,
		httpClient:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

// boolPtr 辅助函数：返回 bool 指针（用于 ChatRequest 可选字段）
func boolPtr(b bool) *bool { return &b }

// Chat 非流式调用（用于需要完整响应再处理的场景）
func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool) (*Message, error) {
	req := ChatRequest{
		Model:       c.Model,
		Messages:    messages,
		Tools:       tools,
		Stream:      false,
		Temperature: c.Temperature,
	}

	slog.Debug("[LLM↑] 发送请求", "messages", len(messages), "tools", len(tools), "model", c.Model)

	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	body, err := io.ReadAll(respBody)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	// 先尝试解析为错误响应
	var errResp ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		return nil, fmt.Errorf("LLM API 错误: %s (type=%s)", errResp.Error.Message, errResp.Error.Type)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 LLM 响应失败: %w\n原始响应: %s", err, string(body))
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM 返回空 choices")
	}

	msg := chatResp.Choices[0].Message
	slog.Debug("[LLM↓] 收到响应",
		"finish_reason", chatResp.Choices[0].FinishReason,
		"content_len", len(msg.Content),
		"tool_calls", len(msg.ToolCalls),
		"total_tokens", chatResp.Usage.TotalTokens,
	)

	return &msg, nil
}

// ChatNoThink 非流式调用，强制关闭思考模式（用于中间步骤，如 HyDE 文档生成）
// 避免推理模型在中间步骤消耗大量时间做思考
func (c *Client) ChatNoThink(ctx context.Context, messages []Message, tools []Tool) (*Message, error) {
	f := false
	req := ChatRequest{
		Model:          c.Model,
		Messages:       messages,
		Tools:          tools,
		Stream:         false,
		Temperature:    c.Temperature,
		EnableThinking: &f,
	}

	slog.Debug("[LLM↑] 发送请求（无思考）", "messages", len(messages), "model", c.Model)

	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	body, err := io.ReadAll(respBody)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	var errResp ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		return nil, fmt.Errorf("LLM API 错误: %s (type=%s)", errResp.Error.Message, errResp.Error.Type)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 LLM 响应失败: %w\n原始响应: %s", err, string(body))
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM 返回空 choices")
	}

	msg := chatResp.Choices[0].Message
	slog.Debug("[LLM↓] 收到响应（无思考）",
		"finish_reason", chatResp.Choices[0].FinishReason,
		"content_len", len(msg.Content),
	)
	return &msg, nil
}

// ChatStream 流式调用，通过 onToken 实时回调 token
// onUsage: 流完成后回调 token 用量（传 nil 则忽略）
// 返回的 Message 包含完整拼接后的 content 和 tool_calls
func (c *Client) ChatStream(ctx context.Context, messages []Message, tools []Tool, onToken func(string), onUsage func(Usage)) (*Message, error) {
	req := ChatRequest{
		Model:         c.Model,
		Messages:      messages,
		Tools:         tools,
		Stream:        true,
		Temperature:   c.Temperature,
		StreamOptions: &StreamOptions{IncludeUsage: true},
	}

	slog.Debug("[LLM↑] 发送流式请求", "messages", len(messages), "tools", len(tools), "model", c.Model)

	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	result, err := ParseSSEStream(respBody, onToken, nil)
	if err != nil {
		return nil, err
	}

	msg := &Message{
		Role:      RoleAssistant,
		Content:   result.Content,
		ToolCalls: result.ToolCalls,
	}

	slog.Debug("[LLM↓] 流式响应完成",
		"content_len", len(msg.Content),
		"tool_calls", len(msg.ToolCalls),
		"total_tokens", result.Usage.TotalTokens,
	)

	if onUsage != nil && result.Usage.TotalTokens > 0 {
		onUsage(result.Usage)
	}

	return msg, nil
}

// ChatStreamEx 流式调用，额外支持 onThinking 回调捕获推理模型的思考过程
// 适用于 Qwen3、DeepSeek-R1 等输出 reasoning_content 的模型
func (c *Client) ChatStreamEx(ctx context.Context, messages []Message, tools []Tool, onToken func(string), onThinking func(string), onUsage func(Usage)) (*Message, *StreamResult, error) {
	req := ChatRequest{
		Model:         c.Model,
		Messages:      messages,
		Tools:         tools,
		Stream:        true,
		Temperature:   c.Temperature,
		StreamOptions: &StreamOptions{IncludeUsage: true},
	}

	slog.Debug("[LLM↑] 发送流式请求（带思考）", "messages", len(messages), "tools", len(tools), "model", c.Model)

	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	defer respBody.Close()

	result, err := ParseSSEStream(respBody, onToken, onThinking)
	if err != nil {
		return nil, nil, err
	}

	msg := &Message{
		Role:      RoleAssistant,
		Content:   result.Content,
		ToolCalls: result.ToolCalls,
	}

	slog.Debug("[LLM↓] 流式响应完成（带思考）",
		"content_len", len(msg.Content),
		"reasoning_len", len(result.ReasoningContent),
		"total_tokens", result.Usage.TotalTokens,
	)

	if onUsage != nil && result.Usage.TotalTokens > 0 {
		onUsage(result.Usage)
	}

	return msg, result, nil
}

// doRequest 发送 HTTP POST 请求，返回响应体（调用方负责关闭）
func (c *Client) doRequest(ctx context.Context, req ChatRequest) (io.ReadCloser, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	if req.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("LLM HTTP 请求失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM API 返回非 200 状态码 %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}
