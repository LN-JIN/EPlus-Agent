// Phase 4: 仿真运行模块
// 将 simulation.Agent.RunWithRepair 封装为 session.PhaseModule 接口，
// 供 Orchestrator 统一调用。

package simulation

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/eplusrun"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/session"
	"energyplus-agent/internal/ui"
)

// Module Phase 4 仿真运行模块
type Module struct {
	agent *Agent
	cfg   *config.Config
}

// New 创建 Phase 4 仿真模块
func New(runner *eplusrun.Runner, llmClient *llm.Client, cfg *config.Config) *Module {
	maxFix := cfg.Modules.Simulation.MaxFixAttempts
	if maxFix <= 0 {
		maxFix = 10
	}
	return &Module{
		agent: NewAgent(runner, llmClient, cfg, maxFix),
		cfg:   cfg,
	}
}

func (m *Module) Name() string { return string(session.PhaseSimRunning) }

// Run 执行仿真，带 ReAct 修复循环
func (m *Module) Run(ctx context.Context, state *session.SessionState) error {
	logger.Phase("阶段 4/6", "仿真运行 — EnergyPlus 仿真（带 LLM 自动修复）")

	if state.IDFPath == "" {
		return fmt.Errorf("IDFPath 为空，Phase 3 必须先完成")
	}

	epwPath := m.cfg.Session.EPWPath
	stem := strings.TrimSuffix(filepath.Base(state.IDFPath), filepath.Ext(state.IDFPath))
	outputBaseDir := filepath.Join(m.cfg.Session.OutputDir, "simulation", stem)

	m.agent.WithLogContext(state.SessionID, m.cfg.Session.OutputDir)
	result, err := m.agent.RunWithRepair(
		ctx,
		state.IDFPath,
		epwPath,
		outputBaseDir,
		state.IntentSummary,
		state,
	)
	if err != nil {
		return fmt.Errorf("仿真运行异常: %w", err)
	}

	if !result.Success {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "仿真失败（未知原因）"
		}
		slog.Warn("[Sim Module] 仿真失败", "fix_attempts", result.FixAttempts, "err", errMsg)
		ui.PrintWarning(fmt.Sprintf("仿真失败（修复 %d 次后仍未成功）: %s", result.FixAttempts, errMsg))
		// 不返回 error — 允许流程继续（Phase 5 仍可读取已有仿真结果）
		state.FailureReason = errMsg
		return nil
	}

	state.IDFPath = result.IDFPath
	state.SimOutDir = result.SimOutDir
	state.AddIDFSnapshot(
		fmt.Sprintf("sim_success_fix%d", result.FixAttempts),
		result.IDFPath,
		result.SimOutDir,
	)

	ui.PrintSuccess(fmt.Sprintf("仿真完成 (修复 %d 次): %s", result.FixAttempts, result.SimOutDir))
	slog.Info("[Sim Module] Phase 4 完成",
		"idf", result.IDFPath,
		"sim_out", result.SimOutDir,
		"fix_attempts", result.FixAttempts,
	)
	return nil
}
