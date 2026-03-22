// LLM 流式响应解析模块
// 负责解析 OpenAI SSE (Server-Sent Events) 格式的流式响应。
// 处理跨多个 chunk 的 tool_calls 拼接（function arguments 分片下发），
// 并在流式接收的同时通过回调实时展示 LLM 思考内容。

package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// StreamResult 流式读取的最终汇总结果
type StreamResult struct {
	Content   string     // 完整文本内容
	ToolCalls []ToolCall // 完整的工具调用列表（拼接后）
}

// ParseSSEStream 从 HTTP 响应体中解析 SSE 流式数据
// onToken: 每收到一个文本 token 时的回调（用于实时打印）
func ParseSSEStream(body io.Reader, onToken func(token string)) (*StreamResult, error) {
	result := &StreamResult{}
	// toolCallsMap 按 index 累积 tool_calls 的 arguments
	toolCallsMap := map[int]*ToolCall{}

	scanner := bufio.NewScanner(body)
	// 增大 buffer 以处理较长的单行数据
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// SSE 格式：每行以 "data: " 开头
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		if data == "" {
			continue
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// 尝试解析为错误响应
			var errResp ErrorResponse
			if jsonErr := json.Unmarshal([]byte(data), &errResp); jsonErr == nil && errResp.Error.Message != "" {
				return nil, fmt.Errorf("LLM API 错误: %s", errResp.Error.Message)
			}
			// 非关键错误，跳过这个 chunk
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// 处理文本内容
		if delta.Content != "" {
			result.Content += delta.Content
			if onToken != nil {
				onToken(delta.Content)
			}
		}

		// 处理 tool_calls（分片拼接）
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			if _, ok := toolCallsMap[idx]; !ok {
				// 首次出现：初始化
				toolCallsMap[idx] = &ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: ToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: "",
					},
				}
			}
			// 累积 arguments 字符串
			toolCallsMap[idx].Function.Arguments += tc.Function.Arguments
			// 更新 ID（有些模型只在第一个 chunk 给 ID）
			if tc.ID != "" {
				toolCallsMap[idx].ID = tc.ID
			}
			if tc.Function.Name != "" {
				toolCallsMap[idx].Function.Name = tc.Function.Name
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("读取流式响应失败: %w", err)
	}

	// 将 map 转为有序切片
	result.ToolCalls = make([]ToolCall, len(toolCallsMap))
	for idx, tc := range toolCallsMap {
		if idx < len(result.ToolCalls) {
			result.ToolCalls[idx] = *tc
		}
	}

	// 过滤空条目
	filtered := result.ToolCalls[:0]
	for _, tc := range result.ToolCalls {
		if tc.Function.Name != "" {
			filtered = append(filtered, tc)
		}
	}
	result.ToolCalls = filtered

	return result, nil
}
