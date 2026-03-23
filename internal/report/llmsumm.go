// LLM 摘要生成
// Summarize: Phase 5 单次仿真结果分析
// SummarizeComparison: Phase 6 多变体对比分析（接受 JSON 字符串，避免循环依赖）

package report

import (
	"context"
	"fmt"
	"strings"

	"energyplus-agent/internal/llm"
)

// Summarize 调用 LLM 一次生成能耗分析摘要
// data: 仿真数据（含汇总指标）
// intentSummary: 建筑意图摘要（提供背景）
// analysisGoal: 分析目标（空字符串使用默认目标）
func Summarize(
	ctx context.Context,
	llmClient *llm.Client,
	data *SimData,
	intentSummary, analysisGoal string,
) (string, int, error) {
	if data == nil {
		return "(no simulation data to analyze)", 0, nil
	}

	goal := analysisGoal
	if goal == "" {
		goal = "总结建筑全年能耗特征，识别主要能耗来源，并提出节能改进建议"
	}

	sysPrompt := `You are a building energy analysis expert. Analyze the EnergyPlus simulation results and provide a clear, actionable summary in Chinese.

Your analysis should:
1. Summarize annual energy consumption by category (heating, cooling, lighting, equipment)
2. Identify the dominant energy loads and their causes
3. Compare performance against typical benchmarks for the building type
4. Provide 2-3 specific, actionable energy-saving recommendations

Use Markdown formatting with headers and bullet points.`

	if intentSummary != "" {
		sysPrompt += "\n\n## 建筑背景\n" + intentSummary
	}

	dataText := FormatSummaryText(data)
	userMsg := fmt.Sprintf("## 分析目标\n%s\n\n## 仿真结果数据\n```\n%s\n```\n\n请根据以上数据生成分析报告：",
		goal, dataText)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: sysPrompt},
		{Role: llm.RoleUser, Content: userMsg},
	}

	var sb strings.Builder
	var usedTokens int
	_, err := llmClient.ChatStream(ctx, messages, nil,
		func(chunk string) { sb.WriteString(chunk) },
		func(u llm.Usage) { usedTokens = u.TotalTokens },
	)
	if err != nil {
		return "", usedTokens, fmt.Errorf("LLM 摘要生成失败: %w", err)
	}

	return strings.TrimSpace(sb.String()), usedTokens, nil
}

// SummarizeComparison 调用 LLM 对 Phase 6 多变体仿真结果进行对比分析
// resultsJSON: json.MarshalIndent([]WorkerResult) 后的字符串（避免 report 包导入 paramanalysis）
// analysisGoal: 用户的参数分析目标
// intentSummary: 建筑意图摘要
func SummarizeComparison(
	ctx context.Context,
	llmClient *llm.Client,
	resultsJSON string,
	analysisGoal, intentSummary string,
) (string, int, error) {
	sysPrompt := `You are a building energy parametric study expert. Analyze the results of multiple EnergyPlus simulation variants and provide a comparative assessment in Chinese.

Your analysis should:
1. Identify the best-performing variant and quantify the improvement vs. baseline
2. Quantify the impact of each parameter change on energy consumption
3. Provide 2-3 actionable design recommendations based on the results
4. Explain any failed variants if present (infer likely causes)

Focus on practical insights for building design decisions. Use Markdown formatting.`

	if intentSummary != "" {
		sysPrompt += "\n\n## 建筑背景\n" + intentSummary
	}

	userMsg := fmt.Sprintf(
		"## 参数分析目标\n%s\n\n## 各变体仿真结果（JSON）\n```json\n%s\n```\n\n请根据以上数据生成对比分析报告：",
		analysisGoal, resultsJSON)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: sysPrompt},
		{Role: llm.RoleUser, Content: userMsg},
	}

	var sb strings.Builder
	var usedTokens int
	_, err := llmClient.ChatStream(ctx, messages, nil,
		func(chunk string) { sb.WriteString(chunk) },
		func(u llm.Usage) { usedTokens = u.TotalTokens },
	)
	if err != nil {
		return "", usedTokens, fmt.Errorf("LLM 对比分析失败: %w", err)
	}

	return strings.TrimSpace(sb.String()), usedTokens, nil
}
