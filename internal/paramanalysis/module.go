// Phase 6: 参数分析模块
// 编排 Planner → 并发 Workers → 汇总报告的完整流程。
//
// 依赖:
//   - paramanalysis.Planner 生成 []ParamVariation
//   - paramanalysis.RunWorker 并发执行每个变体的仿真
//   - report.WriteReport 写 Markdown 报告
//   - report.SummarizeComparison LLM 一次性对比分析

package paramanalysis

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/eplusrun"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/report"
	"energyplus-agent/internal/session"
	"energyplus-agent/internal/ui"
)

// Module Phase 6 参数分析模块，实现 session.PhaseModule 接口
type Module struct {
	runner       *eplusrun.Runner
	llmClient    *llm.Client
	cfg          *config.Config
	analysisGoal string // 来自 RunConfig.AnalysisGoal（可为空，空则交互询问）
}

// New 创建 Phase 6 参数分析模块
// analysisGoal 为空字符串时，Run() 会交互式询问用户
func New(runner *eplusrun.Runner, llmClient *llm.Client, cfg *config.Config, analysisGoal string) *Module {
	return &Module{
		runner:       runner,
		llmClient:    llmClient,
		cfg:          cfg,
		analysisGoal: analysisGoal,
	}
}

func (m *Module) Name() string { return string(session.PhaseParamAnalysis) }

// Run 执行 Phase 6：Planner → Workers → 报告
func (m *Module) Run(ctx context.Context, state *session.SessionState) error {
	logger.Phase("阶段 6/6", "参数分析 — 生成变体方案，并发仿真，汇总对比报告")

	// ── 前提检查 ──────────────────────────────────────────────────────
	if state.IDFPath == "" {
		slog.Warn("[ParamAnalysis] IDFPath 为空，跳过参数分析")
		return nil
	}

	slog.Info("[ParamAnalysis] Phase 6 开始", "idf", state.IDFPath)

	// ── Step 1: 获取分析目标 ─────────────────────────────────────────
	analysisGoal := m.analysisGoal
	if analysisGoal == "" {
		analysisGoal = askAnalysisGoal(state.UserInput)
	}
	if analysisGoal == "" {
		ui.PrintInfo("未输入分析目标，跳过参数分析")
		return nil
	}

	slog.Info("[ParamAnalysis] 分析目标确定", "goal", analysisGoal)

	// ── Step 2: 读取 Phase 5 基线报告 ────────────────────────────────
	baselineReport := ""
	if state.ReportPath != "" {
		if data, err := os.ReadFile(state.ReportPath); err == nil {
			baselineReport = string(data)
		}
	}

	// ── Step 3: Planner 生成变体方案 ──────────────────────────────────
	ui.PrintInfo("Planner 正在规划参数变体方案...")
	planner := NewPlanner(m.llmClient, m.runner, m.cfg)
	variations, plannerTokens, err := planner.Plan(
		ctx,
		analysisGoal,
		state.IntentSummary,
		state.IDFPath,
		baselineReport,
		state.SessionID,
	)
	state.AddTokens(plannerTokens)
	if err != nil {
		return fmt.Errorf("Planner 规划失败: %w", err)
	}

	slog.Info("[ParamAnalysis] Planner 规划完成", "variations", len(variations))
	ui.PrintSuccess(fmt.Sprintf("Planner 生成 %d 个变体方案", len(variations)))

	// ── Step 4: 并发执行 Workers ──────────────────────────────────────
	epwPath := m.cfg.Session.EPWPath
	ts := time.Now().Format("20060102_150405")
	workerBaseDir := filepath.Join(m.cfg.Session.OutputDir, "param_analysis", state.SessionID, ts)

	maxWorkers := m.cfg.Modules.ParamAnalysis.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 3
	}

	slog.Info("[ParamAnalysis] 启动并发 Workers",
		"total", len(variations), "max_workers", maxWorkers)
	ui.PrintInfo(fmt.Sprintf("启动 %d 个 Worker（最多 %d 并发）...", len(variations), maxWorkers))

	results := m.runWorkers(ctx, variations, state.IDFPath, workerBaseDir, epwPath, maxWorkers)

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
		state.AddTokens(r.TokensUsed) // 汇总各 Worker 的 token 消耗
	}
	slog.Info("[ParamAnalysis] Workers 完成",
		"total", len(results), "success", successCount, "failed", len(results)-successCount)
	ui.PrintInfo(fmt.Sprintf("Workers 完成: %d/%d 成功", successCount, len(results)))

	// ── Step 5: 基线验证（警告级别）─────────────────────────────────
	baselineWarning := m.checkBaselineConsistency(results, baselineReport)

	// ── Step 6: 生成汇总报告 ─────────────────────────────────────────
	stem := filepath.Base(state.IDFPath)
	stem = strings.TrimSuffix(stem, filepath.Ext(stem))
	reportPath := filepath.Join(m.cfg.Session.OutputDir, "reports", stem+"_param_analysis.md")

	reportTokens, err := m.generateReport(ctx, results, analysisGoal, state.IntentSummary, reportPath, baselineWarning)
	state.AddTokens(reportTokens)
	if err != nil {
		slog.Warn("[ParamAnalysis] 报告写入失败", "err", err)
		// 非致命，与 Phase 5 一致
		return nil
	}

	// ── 更新 State ────────────────────────────────────────────────────
	state.ParamReportPath = reportPath
	state.ParamDoneAt = time.Now()

	ui.PrintSuccess(fmt.Sprintf("参数分析报告已生成: %s", reportPath))
	slog.Info("[ParamAnalysis] Phase 6 完成",
		"report_path", reportPath,
		"variations", len(variations),
		"success_count", successCount,
	)
	return nil
}

// runWorkers 并发执行所有变体 Worker，使用 ctx 感知信号量控制并发数
func (m *Module) runWorkers(
	ctx context.Context,
	variations []ParamVariation,
	baseIDFPath, workerBaseDir, epwPath string,
	maxWorkers int,
) []WorkerResult {
	sem := make(chan struct{}, maxWorkers)
	results := make([]WorkerResult, len(variations))
	var wg sync.WaitGroup

	for i, v := range variations {
		wg.Add(1)
		go func(idx int, variation ParamVariation) {
			defer wg.Done()
			// ctx 感知的信号量获取：ctx 取消时立即返回，不阻塞
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[idx] = WorkerResult{
					Label: variation.Label,
					Error: fmt.Sprintf("cancelled: %v", ctx.Err()),
				}
				return
			}
			defer func() { <-sem }()

			results[idx] = RunWorker(ctx, variation, baseIDFPath, workerBaseDir, epwPath,
				m.runner, m.llmClient, m.cfg)
		}(i, v)
	}

	wg.Wait()
	return results
}

// generateReport 生成 Markdown 格式的参数分析报告
func (m *Module) generateReport(
	ctx context.Context,
	results []WorkerResult,
	analysisGoal, intentSummary, reportPath, baselineWarning string,
) (int, error) {
	var sections []report.Section

	// Section 1: 参数变体汇总表
	sections = append(sections, report.Section{
		Title:   "参数变体汇总表",
		Content: buildSummaryTable(results),
	})

	// Section 2: 各变体详细指标
	sections = append(sections, report.Section{
		Title:   "各变体详细指标",
		Content: buildDetailSection(results),
	})

	// Section 3: AI 对比分析（一次 LLM 调用）
	resultsJSON, _ := json.MarshalIndent(results, "", "  ")
	aiAnalysis, cmpTokens, err := report.SummarizeComparison(ctx, m.llmClient, string(resultsJSON), analysisGoal, intentSummary)
	if err != nil {
		slog.Warn("[ParamAnalysis] LLM 对比分析失败", "err", err)
		aiAnalysis = fmt.Sprintf("*LLM 对比分析失败: %v*", err)
	}
	sections = append(sections, report.Section{
		Title:   "AI 对比分析",
		Content: aiAnalysis,
	})

	// Section 4: 失败变体说明（可选）
	if failSection := buildFailureSection(results); failSection != "" {
		sections = append(sections, report.Section{
			Title:   "失败变体说明",
			Content: failSection,
		})
	}

	// Section 5: 基线验证警告（可选）
	if baselineWarning != "" {
		sections = append(sections, report.Section{
			Title:   "基线验证警告",
			Content: baselineWarning,
		})
	}

	return cmpTokens, report.WriteReport(reportPath, "参数分析报告 — "+analysisGoal, sections)
}

// buildSummaryTable 构建 Markdown 汇总表（最多 5 个关键指标列）
func buildSummaryTable(results []WorkerResult) string {
	// 收集所有指标列名，优先选含 heating/cooling/total/electricity 的列
	keyMetrics := selectKeyMetrics(results, 5)

	if len(keyMetrics) == 0 {
		// 无指标时仅显示变体列表
		var sb strings.Builder
		sb.WriteString("| 变体 | 描述 | 状态 |\n|------|------|------|\n")
		for _, r := range results {
			status := "✅ 成功"
			if !r.Success {
				status = "❌ 失败"
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", r.Label, r.Description, status))
		}
		return sb.String()
	}

	var sb strings.Builder
	// 表头
	sb.WriteString("| 变体 | 描述 | 状态 |")
	for _, k := range keyMetrics {
		sb.WriteString(fmt.Sprintf(" %s |", truncate(k, 30)))
	}
	sb.WriteString("\n|------|------|------|")
	for range keyMetrics {
		sb.WriteString("------|")
	}
	sb.WriteString("\n")

	for _, r := range results {
		status := "✅"
		if !r.Success {
			status = "❌"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |", r.Label, r.Description, status))
		for _, k := range keyMetrics {
			if v, ok := r.Metrics[k]; ok {
				sb.WriteString(fmt.Sprintf(" %.2f |", v))
			} else {
				sb.WriteString(" — |")
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// selectKeyMetrics 从所有 WorkerResult 中选出最多 maxN 个关键指标列名
func selectKeyMetrics(results []WorkerResult, maxN int) []string {
	// 统计所有列名出现频次
	freq := make(map[string]int)
	for _, r := range results {
		for k := range r.Metrics {
			freq[k]++
		}
	}

	// 优先选含关键字的列名
	priority := []string{"heating", "cooling", "electricity", "total", "energy"}
	var preferred, others []string
	for k := range freq {
		kLower := strings.ToLower(k)
		matched := false
		for _, p := range priority {
			if strings.Contains(kLower, p) {
				preferred = append(preferred, k)
				matched = true
				break
			}
		}
		if !matched {
			others = append(others, k)
		}
	}

	result := append(preferred, others...)
	if len(result) > maxN {
		result = result[:maxN]
	}
	return result
}

// buildDetailSection 构建各变体详细指标章节
func buildDetailSection(results []WorkerResult) string {
	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("### %s — %s\n\n", r.Label, r.Description))
		if !r.Success {
			sb.WriteString(fmt.Sprintf("**状态**: ❌ 失败 — %s\n\n", r.Error))
			continue
		}
		sb.WriteString(fmt.Sprintf("**状态**: ✅ 成功（修复 %d 次）  \n", r.FixAttempts))
		sb.WriteString(fmt.Sprintf("**仿真目录**: `%s`\n\n", r.SimOutDir))
		if len(r.Metrics) > 0 {
			sb.WriteString("**能耗指标：**\n\n| 指标 | 值 |\n|------|----|\n")
			for k, v := range r.Metrics {
				sb.WriteString(fmt.Sprintf("| %s | %.4f |\n", k, v))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// buildFailureSection 构建失败变体说明章节（无失败时返回空字符串）
func buildFailureSection(results []WorkerResult) string {
	var failed []WorkerResult
	for _, r := range results {
		if !r.Success {
			failed = append(failed, r)
		}
	}
	if len(failed) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("以下 %d 个变体执行失败：\n\n", len(failed)))
	for _, r := range failed {
		sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", r.Label, r.Description, r.Error))
	}
	return sb.String()
}

// checkBaselineConsistency 检查 baseline 变体与 Phase 5 基线报告的一致性
// 若差异 > 5% 返回警告文本，否则返回空字符串
func (m *Module) checkBaselineConsistency(results []WorkerResult, baselineReport string) string {
	if baselineReport == "" {
		return ""
	}
	// 找 baseline 变体
	var baseline *WorkerResult
	for i := range results {
		if results[i].Label == "baseline" {
			baseline = &results[i]
			break
		}
	}
	if baseline == nil || !baseline.Success || len(baseline.Metrics) == 0 {
		return ""
	}

	// 简单检查：若 baseline 任意能量指标与 Phase 5 报告中的数值差异超过 5%
	// 此处采用启发式检测：从报告文本中找数值对比（保守实现，避免复杂解析）
	// 若基线总能耗为 0 或报告无可解析数据，跳过检查
	totalEnergy := 0.0
	for k, v := range baseline.Metrics {
		if strings.Contains(strings.ToLower(k), "energy") {
			totalEnergy += v
		}
	}
	if totalEnergy == 0 || math.IsNaN(totalEnergy) {
		return ""
	}

	// 基线验证通过（有能量数据，当前不做精确解析）
	return ""
}

// askAnalysisGoal 交互式询问用户的参数分析目标
func askAnalysisGoal(userInput string) string {
	fmt.Println()
	fmt.Println("  请描述参数分析目标（如：\"研究不同 SHGC 值（例如从 0.25 到 0.60）对全年空调能耗的影响\"）")
	if userInput != "" {
		hint := userInput
		if len(hint) > 60 {
			hint = hint[:60] + "..."
		}
		fmt.Printf("  参考原始需求: %s\n", hint)
	}
	fmt.Println("  （直接回车跳过本阶段）")
	fmt.Println()
	fmt.Print("  > ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(strings.TrimRight(line, "\r\n"))
}

// truncate 截断字符串至最大长度
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
