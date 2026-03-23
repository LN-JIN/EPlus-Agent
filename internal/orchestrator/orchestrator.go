// Orchestrator 主流程编排模块
// 驱动 PhaseModule 列表依次执行，支持从任意阶段进入（灵活入口）。
// 持有所有基础设施对象（LLM 客户端、MCP 客户端、Runner），
// 在 RunWithConfig 时按需初始化各模块。

package orchestrator

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/eplusrun"
	"energyplus-agent/internal/idfconvert"
	"energyplus-agent/internal/intent"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/mcp"
	"energyplus-agent/internal/paramanalysis"
	"energyplus-agent/internal/rag"
	"energyplus-agent/internal/report"
	"energyplus-agent/internal/session"
	"energyplus-agent/internal/simulation"
	"energyplus-agent/internal/skills"
	"energyplus-agent/internal/ui"
)

// RunConfig 灵活入口配置
// 通过设置不同字段，可从任意阶段开始执行：
//   - 只设 UserInput → Phase 1 完整流程
//   - 设 YAMLPath → 跳过 Phase 1/2，从 Phase 3（YAML→IDF）开始
//   - 设 IDFPath → 跳过 Phase 1-3，从 Phase 4（仿真）开始
//   - 设 SimOutDir → 跳过 Phase 1-4，从 Phase 5（报告）开始
type RunConfig struct {
	UserInput    string // Phase 1: 建筑自然语言描述
	YAMLPath     string // 跳到 Phase 3: 已有 YAML 路径
	IDFPath      string // 跳到 Phase 4: 已有 IDF 路径
	SimOutDir    string // 跳到 Phase 5: 已有仿真输出目录
	EPWPath      string // 自定义 EPW 气象文件（覆盖 config 默认值）
	SkipReport   bool   // 跳过 Phase 5（报告解读）
	SkipParam    bool   // 跳过 Phase 6（参数分析）
	ResumeID     string // 续传会话 ID（从已有 session JSON 恢复）
	AnalysisGoal string // Phase 6 参数分析目标（空字符串则在 Phase 6 内交互询问）
}

// Orchestrator 主流程协调器
type Orchestrator struct {
	cfg         *config.Config
	llmClient   *llm.Client
	mcpClient   *mcp.Client
	runner      *eplusrun.Runner
	retriever   rag.Retriever
	skillLoader *skills.Loader
}

// New 创建 Orchestrator
func New(cfg *config.Config) *Orchestrator {
	llmClient := llm.NewClient(
		cfg.LLM.BaseURL,
		cfg.LLM.APIKey,
		cfg.LLM.Model,
		cfg.LLM.TimeoutSec,
		cfg.LLM.Temperature,
	)

	mcpClient := mcp.NewClient(cfg.MCP.BaseURL, cfg.MCP.TimeoutSec)
	runner := eplusrun.NewRunner(cfg.Session.SimulationScript, cfg.Session.PythonPath)
	skillLoader := skills.Load("skills")

	return &Orchestrator{
		cfg:         cfg,
		llmClient:   llmClient,
		mcpClient:   mcpClient,
		runner:      runner,
		retriever:   &rag.NoopRetriever{},
		skillLoader: skillLoader,
	}
}

// Run 从建筑描述开始完整流程（向后兼容旧接口）
func (o *Orchestrator) Run(ctx context.Context, userInput string) error {
	return o.RunWithConfig(ctx, RunConfig{UserInput: userInput})
}

// RunWithConfig 按 RunConfig 指定的入口执行流程
func (o *Orchestrator) RunWithConfig(ctx context.Context, cfg RunConfig) error {
	// ── 初始化或恢复 Session ─────────────────────────────────────────────
	var state *session.SessionState

	if cfg.ResumeID != "" {
		var err error
		state, err = session.LoadFromFile(o.cfg.Session.OutputDir, cfg.ResumeID)
		if err != nil {
			slog.Warn("[Orch] 恢复会话失败，新建会话", "err", err)
		} else {
			ui.PrintInfo(fmt.Sprintf("已恢复会话: %s (上次阶段: %s)", cfg.ResumeID, state.Phase))
		}
	}
	if state == nil {
		sessionID := fmt.Sprintf("session_%s", time.Now().Format("20060102_150405"))
		state = session.NewSessionState(sessionID)
	}

	// 根据入口参数跳过已完成的阶段
	if cfg.YAMLPath != "" {
		state.Phase = session.PhaseIDFConverting
		state.YAMLPath = cfg.YAMLPath
		ui.PrintInfo(fmt.Sprintf("从 YAML 路径开始: %s", cfg.YAMLPath))
	}
	if cfg.IDFPath != "" {
		state.Phase = session.PhaseSimRunning
		state.IDFPath = cfg.IDFPath
		ui.PrintInfo(fmt.Sprintf("从 IDF 路径开始: %s", cfg.IDFPath))
	}
	if cfg.SimOutDir != "" {
		state.Phase = session.PhaseReportReading
		state.SimOutDir = cfg.SimOutDir
		ui.PrintInfo(fmt.Sprintf("从仿真目录开始: %s", cfg.SimOutDir))
	}
	state.UserInput = cfg.UserInput

	slog.Info("[Orch] 会话开始", "session_id", state.SessionID, "phase", state.Phase)

	// ── 初始化 MCP Server 连接（连接失败不阻断流程）──────────────────────
	if state.Phase == session.PhaseIntentCollection || state.Phase == session.PhaseYAMLGenerating {
		ui.PrintInfo("连接 MCP Server...")
		mcpCtx, mcpCancel := context.WithTimeout(ctx, time.Duration(o.cfg.MCP.InitTimeoutSec)*time.Second)
		if err := o.mcpClient.Initialize(mcpCtx); err != nil {
			ui.PrintWarning("MCP Server 连接失败（继续运行）")
			slog.Warn("[Orch] MCP 初始化失败", "err", err)
		} else {
			ui.PrintSuccess("MCP Server 连接成功")
			_ = o.mcpClient.ClearAll(ctx)
		}
		mcpCancel()
	}

	// ── 环境预检（需要调用 Python 的阶段才检查）────────────────────────────
	if shouldRunPhase(state.Phase, session.PhaseIDFConverting) {
		if err := o.runner.Probe(); err != nil {
			ui.PrintError(fmt.Sprintf("环境检查失败: %v", err))
			return err
		}
	}

	// ── 构建 PhaseModule 列表 ────────────────────────────────────────────
	modules := o.buildModules(cfg)

	// ── 执行各阶段 ───────────────────────────────────────────────────────
	for _, mod := range modules {
		// 跳过已完成的阶段
		if !shouldRunPhase(state.Phase, session.Phase(mod.Name())) {
			slog.Debug("[Orch] 跳过已完成阶段", "phase", mod.Name())
			continue
		}

		// 跳过用户禁用的阶段
		if cfg.SkipReport && mod.Name() == string(session.PhaseReportReading) {
			slog.Info("[Orch] 跳过报告阶段（用户设置）")
			continue
		}
		if cfg.SkipParam && mod.Name() == string(session.PhaseParamAnalysis) {
			slog.Info("[Orch] 跳过参数分析阶段（用户设置）")
			continue
		}

		// Phase 5/6 之前询问用户
		if mod.Name() == string(session.PhaseReportReading) && state.SimOutDir != "" {
			choice := askContinueAfterSim()
			if choice == "skip_all" {
				break
			}
			if choice == "skip_report" {
				continue
			}
		}

		// 执行阶段
		state.Phase = session.Phase(mod.Name())
		if err := mod.Run(ctx, state); err != nil {
			if ctx.Err() != nil {
				ui.PrintWarning("操作已中断")
				return nil
			}
			if strings.Contains(err.Error(), "取消") {
				ui.PrintWarning("用户取消了操作")
				state.Phase = session.PhaseCancelled
				return nil
			}
			state.Phase = session.PhaseFailed
			state.FailureReason = err.Error()
			ui.PrintError(fmt.Sprintf("阶段 %s 失败: %v", mod.Name(), err))
			_ = state.SaveToFile(o.cfg.Session.OutputDir)
			return fmt.Errorf("阶段 %s 失败: %w", mod.Name(), err)
		}

		// 阶段完成后打印 token 消耗汇总
		phaseTokens := state.PhaseTokens[session.Phase(mod.Name())]
		logger.TokenSummary(mod.Name(), phaseTokens, state.TotalTokens)

		// 阶段完成后持久化状态
		_ = state.SaveToFile(o.cfg.Session.OutputDir)
		slog.Info("[Orch] 阶段完成", "phase", mod.Name())
	}

	// ── 展示最终结果 ─────────────────────────────────────────────────────
	state.Phase = session.PhaseDone
	_ = state.SaveToFile(o.cfg.Session.OutputDir)

	totalDuration := state.TotalDuration().Round(time.Second).String()
	ui.PrintFinalResult(state.YAMLPath, state.IDFPath, state.SimOutDir, state.ReportPath, totalDuration)

	slog.Info("[Orch] 会话完成",
		"session_id", state.SessionID,
		"total_duration", totalDuration,
		"yaml_path", state.YAMLPath,
		"idf_path", state.IDFPath,
		"sim_out", state.SimOutDir,
		"report", state.ReportPath,
		"param_report", state.ParamReportPath,
	)
	return nil
}

// buildModules 按顺序构建所有 PhaseModule
func (o *Orchestrator) buildModules(runCfg RunConfig) []session.PhaseModule {
	// 使用自定义 EPW 覆盖 config（不修改全局 config）
	cfgCopy := *o.cfg
	if runCfg.EPWPath != "" {
		cfgCopy.Session.EPWPath = runCfg.EPWPath
	}
	cfg := &cfgCopy

	return []session.PhaseModule{
		intent.NewCollectModule(o.llmClient, o.retriever, o.skillLoader, cfg),
		intent.NewGenerateModule(o.llmClient, cfg),
		idfconvert.New(o.runner, o.llmClient, cfg),
		simulation.New(o.runner, o.llmClient, cfg),
		report.New(o.llmClient, cfg),
		paramanalysis.New(o.runner, o.llmClient, cfg, runCfg.AnalysisGoal),
	}
}

// shouldRunPhase 判断当前阶段是否应该执行
// currentPhase: 当前 session 所在阶段（入口阶段）
// modulePhase: 正在考虑执行的模块阶段
func shouldRunPhase(currentPhase, modulePhase session.Phase) bool {
	order := []session.Phase{
		session.PhaseIntentCollection,
		session.PhaseYAMLGenerating,
		session.PhaseIDFConverting,
		session.PhaseSimRunning,
		session.PhaseReportReading,
		session.PhaseParamAnalysis,
	}
	currentIdx := -1
	moduleIdx := -1
	for i, p := range order {
		if p == currentPhase {
			currentIdx = i
		}
		if p == modulePhase {
			moduleIdx = i
		}
	}
	if currentIdx < 0 || moduleIdx < 0 {
		return true
	}
	return moduleIdx >= currentIdx
}

// askContinueAfterSim 仿真完成后询问用户是否继续后续分析
// 返回: "continue"(继续报告) | "skip_report"(跳过报告做参数分析) | "skip_all"(结束)
func askContinueAfterSim() string {
	ui.PrintSection("仿真完成！请选择后续操作")
	fmt.Println("  [1] 报告解读（读取仿真结果，生成能耗汇总报告）")
	fmt.Println("  [2] 直接结束")
	fmt.Println()
	fmt.Print("  请选择 (1/2，默认 1): ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)

	switch line {
	case "2":
		return "skip_all"
	default:
		return "continue"
	}
}

// State 返回当前会话状态（只读，兼容旧代码）
// Deprecated: 请直接使用 RunWithConfig 并通过 session 包管理状态
func (o *Orchestrator) State() *session.SessionState {
	return nil
}
