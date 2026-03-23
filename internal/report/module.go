// Phase 5: 报告解读模块
// 读取仿真结果（CSV/ESO），调用 LLM 生成 Markdown 分析报告。
//
// 接口预留扩展空间：analysisGoal 参数支持用户指定分析目标，
// 未来可将实现升级为 Plan-Execute-Replan 模型而无需修改接口。

package report

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/session"
	"energyplus-agent/internal/ui"
)

// Module Phase 5 报告解读模块
type Module struct {
	llmClient *llm.Client
	cfg       *config.Config
}

// New 创建 Phase 5 报告模块
func New(llmClient *llm.Client, cfg *config.Config) *Module {
	return &Module{llmClient: llmClient, cfg: cfg}
}

func (m *Module) Name() string { return string(session.PhaseReportReading) }

// Run 读取仿真结果，生成 Markdown 报告
// 分析目标默认为"汇总能耗和温度"，可通过 state 的 IntentSummary 提供背景
func (m *Module) Run(ctx context.Context, state *session.SessionState) error {
	logger.Phase("阶段 5/6", "报告解读 — 读取仿真结果，生成能耗分析报告")

	if state.SimOutDir == "" {
		slog.Warn("[Report] SimOutDir 为空，跳过报告生成")
		return nil
	}

	// 读取仿真数据
	data, err := ReadSimData(state.SimOutDir)
	if err != nil {
		slog.Warn("[Report] 读取仿真数据失败", "err", err)
		ui.PrintWarning(fmt.Sprintf("无法读取仿真数据: %v", err))
		return nil // 不阻断流程
	}

	slog.Info("[Report] 仿真数据读取成功", "source", data.Source, "summary_keys", len(data.Summary))

	// LLM 一次性生成分析摘要
	analysis, sumTokens, err := Summarize(ctx, m.llmClient, data, state.IntentSummary, "")
	state.AddTokens(sumTokens)
	if err != nil {
		slog.Warn("[Report] LLM 分析失败", "err", err)
		analysis = fmt.Sprintf("*LLM 分析失败: %v*\n\n**原始数据摘要:**\n\n```\n%s\n```",
			err, FormatSummaryText(data))
	}

	// 确定报告路径
	stem := filepath.Base(state.SimOutDir)
	if state.YAMLPath != "" {
		stem = strings.TrimSuffix(filepath.Base(state.YAMLPath), filepath.Ext(state.YAMLPath))
	}
	reportPath := filepath.Join(m.cfg.Session.OutputDir, "reports", stem+"_report.md")

	// 写入 Markdown 报告
	sections := []Section{
		{Title: "AI 能耗分析", Content: analysis},
		{Title: "原始数据摘要", Content: "```\n" + FormatSummaryText(data) + "\n```"},
	}

	buildingTitle := stem
	if state.IntentSummary != "" {
		// 从摘要第一行提取建筑名称
		firstLine := strings.SplitN(state.IntentSummary, "\n", 2)[0]
		buildingTitle = firstLine
	}

	if writeErr := WriteReport(reportPath, "能耗分析报告 — "+buildingTitle, sections); writeErr != nil {
		slog.Warn("[Report] 写入报告失败", "err", writeErr)
		return nil
	}

	state.ReportPath = reportPath
	ui.PrintSuccess(fmt.Sprintf("报告已生成: %s", reportPath))
	slog.Info("[Report] Phase 5 完成", "report_path", reportPath)
	return nil
}
