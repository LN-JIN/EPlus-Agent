// ReAct Agent 核心模块
// 实现 Reasoning + Acting 循环：LLM 思考 → 决定调用工具 → 获取 Observation → 继续思考。
// 所有 LLM 输出采用流式展示，工具调用入参/出参均通过日志模块完整记录。
// 本模块是意图收集、YAML 生成、MCP 错误自愈等所有智能行为的统一底层引擎。

package react

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/tools"
)

// Agent ReAct 智能体
type Agent struct {
	llmClient *llm.Client
	registry  *tools.Registry
	maxIter   int
}

// NewAgent 创建 ReAct Agent
func NewAgent(llmClient *llm.Client, registry *tools.Registry, maxIter int) *Agent {
	if maxIter <= 0 {
		maxIter = 15
	}
	return &Agent{
		llmClient: llmClient,
		registry:  registry,
		maxIter:   maxIter,
	}
}

// Run 执行 ReAct 循环
// systemPrompt: 系统提示词（定义 Agent 的角色和目标）
// userInput: 用户输入（任务描述）
// 返回 Result 包含完整步骤记录和最终答案
func (a *Agent) Run(ctx context.Context, systemPrompt, userInput string) (*Result, error) {
	result := &Result{Steps: make([]Step, 0)}

	// 初始化对话历史
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
		{Role: llm.RoleUser, Content: userInput},
	}

	llmTools := a.registry.ToLLMTools()

	slog.Debug("[ReAct] 开始执行",
		"max_iter", a.maxIter,
		"tools", a.registry.Names(),
	)

	for iter := 1; iter <= a.maxIter; iter++ {
		step := Step{Iter: iter}

		slog.Debug("[ReAct] 开始第 N 轮迭代", "iter", iter, "messages", len(messages))
		fmt.Printf("\n") // 保证流式输出前有换行

		// 流式调用 LLM，实时打印 token
		respMsg, err := a.llmClient.ChatStream(ctx, messages, llmTools, func(token string) {
			logger.LLMThought(token)
		})
		if err != nil {
			result.Error = fmt.Errorf("第 %d 轮 LLM 调用失败: %w", iter, err)
			return result, result.Error
		}
		logger.LLMThoughtEnd()

		step.Thought = respMsg.Content

		// ── 情况 A：LLM 调用了工具 ──────────────────────────────────
		if len(respMsg.ToolCalls) > 0 {
			// 将 assistant 消息（含 tool_calls）加入历史
			messages = append(messages, *respMsg)

			// 顺序执行所有工具调用
			for _, tc := range respMsg.ToolCalls {
				step.Action = tc.Function.Name
				step.ActionInput = tc.Function.Arguments

				// 格式化参数用于展示
				prettyArgs := formatArgs(tc.Function.Arguments)
				logger.ToolCall(tc.Function.Name, prettyArgs)
				slog.Debug("[ReAct] 执行工具",
					"tool", tc.Function.Name,
					"args", tc.Function.Arguments,
				)

				observation, execErr := a.registry.Execute(tc.Function.Name, tc.Function.Arguments)
				if execErr != nil {
					observation = fmt.Sprintf("ERROR: %v", execErr)
				}

				step.Observation = observation
				logger.ToolResult(tc.Function.Name, observation)
				slog.Debug("[ReAct] 工具返回",
					"tool", tc.Function.Name,
					"result_len", len(observation),
				)

				// 将 tool 结果加入对话历史
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    observation,
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
				})
			}

			result.Steps = append(result.Steps, step)
			continue // 进入下一轮迭代
		}

		// ── 情况 B：LLM 直接给出最终回答 ────────────────────────────
		step.IsFinal = true
		step.FinalAnswer = respMsg.Content
		result.Steps = append(result.Steps, step)
		result.FinalAnswer = respMsg.Content

		slog.Info("[ReAct] 完成", "iters", iter, "answer_len", len(result.FinalAnswer))
		return result, nil
	}

	// 超过最大迭代次数
	result.Error = fmt.Errorf("ReAct 超过最大迭代次数 %d，未得到最终答案", a.maxIter)
	slog.Warn("[ReAct] 迭代超限", "max_iter", a.maxIter)
	return result, result.Error
}

// RunWithMessages 支持传入自定义初始消息列表（用于多轮对话续接）
func (a *Agent) RunWithMessages(ctx context.Context, messages []llm.Message) (*Result, error) {
	result := &Result{Steps: make([]Step, 0)}
	llmTools := a.registry.ToLLMTools()

	for iter := 1; iter <= a.maxIter; iter++ {
		step := Step{Iter: iter}
		fmt.Printf("\n")

		respMsg, err := a.llmClient.ChatStream(ctx, messages, llmTools, func(token string) {
			logger.LLMThought(token)
		})
		if err != nil {
			result.Error = fmt.Errorf("第 %d 轮 LLM 调用失败: %w", iter, err)
			return result, result.Error
		}
		logger.LLMThoughtEnd()

		step.Thought = respMsg.Content

		if len(respMsg.ToolCalls) > 0 {
			messages = append(messages, *respMsg)

			for _, tc := range respMsg.ToolCalls {
				step.Action = tc.Function.Name
				step.ActionInput = tc.Function.Arguments

				logger.ToolCall(tc.Function.Name, formatArgs(tc.Function.Arguments))
				observation, _ := a.registry.Execute(tc.Function.Name, tc.Function.Arguments)
				step.Observation = observation
				logger.ToolResult(tc.Function.Name, observation)

				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    observation,
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
				})
			}

			result.Steps = append(result.Steps, step)
			continue
		}

		step.IsFinal = true
		step.FinalAnswer = respMsg.Content
		result.Steps = append(result.Steps, step)
		result.FinalAnswer = respMsg.Content
		return result, nil
	}

	result.Error = fmt.Errorf("ReAct 超过最大迭代次数 %d", a.maxIter)
	return result, result.Error
}

// formatArgs 将 JSON 参数字符串格式化为可读形式（用于日志展示）
func formatArgs(argsJSON string) string {
	if argsJSON == "" {
		return "{}"
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return argsJSON
	}
	// 简单截断长字符串值
	for k, v := range m {
		if s, ok := v.(string); ok && len(s) > 80 {
			m[k] = s[:80] + "..."
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}
