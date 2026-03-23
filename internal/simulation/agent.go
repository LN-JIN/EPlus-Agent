// 仿真 ReAct Agent（可复用核心）
// RunWithRepair 是 Phase 4 和 Phase 6 Worker 共用的仿真+修复循环：
//   仿真 → 检查结果 → 若失败 LLM ReAct 分析 err 文件 → 修改 IDF → 重试
//
// 通过 simulation.Agent 封装使 Phase 6 Worker 不依赖 SessionState，
// 只通过参数传递所需信息，便于并发执行。

package simulation

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/eplusrun"
	"energyplus-agent/internal/fault"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/react"
	"energyplus-agent/internal/session"
	"energyplus-agent/internal/tools"
)

// SimResult 单次仿真+修复的结果
type SimResult struct {
	IDFPath     string             // 最终使用的 IDF 路径
	SimOutDir   string             // 仿真输出目录
	Success     bool               // 仿真是否成功
	FixAttempts int                // 实际修复尝试次数
	TotalTokens int                // 本次仿真+修复累计 token 消耗
	Metrics     map[string]float64 // 提取的指标（Phase 6 使用）
	Error       string             // 最终错误信息（Success=false 时有效）
}

// Agent 可复用的仿真+修复 ReAct Agent
type Agent struct {
	runner    *eplusrun.Runner
	llmClient *llm.Client
	maxFix    int
	cfg       *config.Config
	// 用于 ReAct 日志写入（可选，空字符串则跳过）
	sessionID string
	outputDir string
}

// NewAgent 创建仿真 Agent
func NewAgent(
	runner *eplusrun.Runner,
	llmClient *llm.Client,
	cfg *config.Config,
	maxFix int,
) *Agent {
	return &Agent{
		runner:    runner,
		llmClient: llmClient,
		maxFix:    maxFix,
		cfg:       cfg,
	}
}

// WithLogContext 设置 ReAct 日志的 sessionID 和 outputDir
func (a *Agent) WithLogContext(sessionID, outputDir string) *Agent {
	a.sessionID = sessionID
	a.outputDir = outputDir
	return a
}

// RunWithRepair 执行仿真并在失败时用 LLM ReAct 循环修复 IDF
// idfPath: 要仿真的 IDF 文件路径
// epwPath: EPW 气象文件
// outputBaseDir: 仿真输出根目录（每次仿真会追加版本号子目录）
// intentSummary: 建筑意图摘要（注入 SystemPrompt，让 LLM 了解背景）
// state: 可为 nil（Phase 6 Worker 不持有 SessionState）
func (a *Agent) RunWithRepair(
	ctx context.Context,
	idfPath, epwPath, outputBaseDir, intentSummary string,
	state *session.SessionState,
) (*SimResult, error) {
	result := &SimResult{IDFPath: idfPath}

	snapshotDir := filepath.Join(outputBaseDir, "snapshots")
	maxFix := a.maxFix
	if maxFix <= 0 {
		maxFix = 10
	}

	guard := &fault.SameErrorGuard{MaxRepeat: 3}

	for attempt := 1; attempt <= maxFix+1; attempt++ {
		simOutDir := filepath.Join(outputBaseDir, fmt.Sprintf("v%d", attempt))

		// 执行仿真
		outDir, simErr := a.runner.RunSimulation(ctx, idfPath, epwPath, simOutDir)
		if outDir != "" {
			result.SimOutDir = outDir
		}
		if state != nil && outDir != "" {
			state.SimOutDir = outDir
		}

		// 检查仿真结果
		checkResult, _ := tools.CheckSimulationResult(simOutDir)
		if simErr == nil && (checkResult == nil || checkResult.Success) {
			result.IDFPath = idfPath
			result.Success = true
			result.FixAttempts = attempt - 1
			return result, nil
		}

		// 环境致命错误（EPW 缺失、EnergyPlus 不可用等）— 立即终止
		if fault.IsFatal(simErr) {
			slog.Error("[Sim Agent] 检测到环境致命错误，终止修复", "err", simErr)
			result.Error = fmt.Sprintf("环境配置错误（无法自动修复）: %v", simErr)
			return result, nil
		}

		// 输出目录不存在 → 仿真进程根本未启动，属于运行环境问题，修改 IDF 无意义
		if checkResult != nil && !checkResult.DirExists {
			slog.Error("[Sim Agent] 仿真输出目录未创建，疑似运行环境问题，终止修复",
				"expected_dir", simOutDir, "sim_err", simErr)
			errMsg := "仿真输出目录未创建，仿真进程可能未正常启动。请检查：\n" +
				"  1. EnergyPlus 是否已安装并可执行\n" +
				"  2. EPW 气象文件路径是否正确\n" +
				"  3. config.yaml 中 simulation_script / python_path 是否配置正确"
			if simErr != nil {
				errMsg += "\n原始错误: " + simErr.Error()
			}
			result.Error = errMsg
			return result, nil
		}

		// 仿真失败且超过修复上限
		if attempt > maxFix {
			errMsg := fmt.Sprintf("仿真失败（已尝试 %d 次修复）", maxFix)
			if simErr != nil {
				errMsg += ": " + simErr.Error()
			} else if checkResult != nil {
				errMsg += "\n" + checkResult.ErrSummary
			}
			result.Error = errMsg
			return result, nil // 不返回 error，由调用方判断 result.Success
		}

		slog.Info(fmt.Sprintf("[Sim Agent] 仿真修复 %d/%d", attempt, maxFix), "sim_err", simErr)

		// 空转检测：用 EnergyPlus 错误摘要（比 simErr 更精确）做指纹
		errFingerprint := ""
		if checkResult != nil && checkResult.ErrSummary != "" {
			errFingerprint = checkResult.ErrSummary
		} else if simErr != nil {
			errFingerprint = simErr.Error()
		}
		spinning, hint := guard.Observe(errFingerprint)
		if spinning {
			slog.Warn("[Sim Agent] 检测到修复空转，终止重试", "hint", hint)
			result.Error = fmt.Sprintf("仿真修复空转（相同错误连续出现）: %s", errFingerprint)
			return result, nil
		}

		// LLM ReAct 修复（将空转提示注入 prompt）
		fixedIDF, repairTokens, fixErr := a.repairWithReAct(ctx, idfPath, snapshotDir, simOutDir, intentSummary, attempt, hint, state)
		result.TotalTokens += repairTokens
		if fixErr != nil {
			slog.Warn("[Sim Agent] ReAct 修复失败，停止重试", "err", fixErr)
			result.Error = fixErr.Error()
			return result, nil
		}

		// 保存修复后 IDF 的快照
		idfPath = fixedIDF
		result.FixAttempts = attempt
	}

	// 将本次仿真+修复消耗的 token 写入 state（Phase 4 实参；Phase 6 Worker 传 nil，由 Worker 从 result 读取）
	if state != nil && result.TotalTokens > 0 {
		state.AddTokens(result.TotalTokens)
	}

	return result, nil
}

// repairWithReAct 通过 ReAct Agent 分析仿真错误并修复 IDF
// spinningHint: 若非空，表示检测到空转，将注入 userMsg 提示 LLM 换策略
func (a *Agent) repairWithReAct(
	ctx context.Context,
	idfPath, snapshotDir, simOutDir, intentSummary string,
	attempt int,
	spinningHint string,
	state *session.SessionState,
) (string, int, error) {
	registry := tools.NewRegistry()

	// 为此次修复创建独立 snapshotDir
	snapDir := filepath.Join(snapshotDir, fmt.Sprintf("attempt_%d", attempt))

	// 当 state 为 nil 时（Phase 6 Worker），用空 state 避免 nil 指针
	stateProxy := state
	if stateProxy == nil {
		stateProxy = session.NewSessionState("worker")
	}

	registerSimTools(registry, a.runner, stateProxy, a.cfg.Session.EPWPath, snapDir)

	maxIter := a.cfg.Modules.Simulation.MaxFixAttempts
	if maxIter <= 0 {
		maxIter = 10
	}
	agent := react.NewAgent(a.llmClient, registry, maxIter)

	systemPrompt := buildSimRepairPrompt(registry, intentSummary)

	checkResult, _ := tools.CheckSimulationResult(simOutDir)
	errSummary := ""
	if checkResult != nil {
		errSummary = checkResult.ErrSummary
	}

	userMsg := fmt.Sprintf(`EnergyPlus simulation failed. Please diagnose the error and fix the IDF file.

## Current IDF Path
%s

## Simulation Output Directory
%s

## Error Summary (from eplusout.err)
%s

Workflow:
1. Call check_simulation_result to review the full error details
2. Call read_idf_object to examine the problematic object(s)
3. Call edit_idf_object to fix the field(s)
4. Call save_idf_snapshot to preserve this fix
5. Call run_simulation to verify the fix works
6. If still failing, repeat from step 1`, idfPath, simOutDir, errSummary)

	if spinningHint != "" {
		userMsg += "\n\n## 注意\n" + spinningHint
	}

	result, reactErr := agent.Run(ctx, systemPrompt, userMsg)

	repairTokens := 0
	if result != nil {
		repairTokens = result.TotalTokens
	}

	// 写 ReAct 日志
	if a.sessionID != "" && a.outputDir != "" && result != nil && len(result.Steps) > 0 {
		logPath := filepath.Join(a.outputDir, "react_logs",
			fmt.Sprintf("%s_simulation_repair_%d.md", a.sessionID, attempt))
		steps := make([]logger.ReActStep, 0, len(result.Steps))
		for _, s := range result.Steps {
			steps = append(steps, logger.ReActStep{
				Iter:        s.Iter,
				Thought:     s.Thought,
				Action:      s.Action,
				ActionInput: s.ActionInput,
				Observation: s.Observation,
				IsFinal:     s.IsFinal,
				FinalAnswer: s.FinalAnswer,
			})
		}
		if writeErr := logger.WriteReActLog(logPath, "simulation_repair", a.sessionID, steps); writeErr != nil {
			slog.Warn("[Sim Agent] ReAct 日志写入失败", "err", writeErr)
		}
	}

	if reactErr != nil {
		return idfPath, repairTokens, fmt.Errorf("ReAct 修复失败: %w", reactErr)
	}

	// 返回最新 IDF 路径（如果 state 已更新则用 state 的，否则用原始路径）
	if stateProxy.IDFPath != "" && stateProxy != state {
		// worker proxy state 更新的 IDFPath
		return stateProxy.IDFPath, repairTokens, nil
	}
	if state != nil && state.IDFPath != "" {
		return state.IDFPath, repairTokens, nil
	}
	return idfPath, repairTokens, nil
}

// buildSimRepairPrompt 构建仿真修复 Agent 的 System Prompt
func buildSimRepairPrompt(registry *tools.Registry, intentSummary string) string {
	var sb strings.Builder
	sb.WriteString(`You are an EnergyPlus simulation expert. Your task is to diagnose simulation errors and fix the IDF file so the simulation completes successfully.

## Strategy
1. Read eplusout.err via check_simulation_result to identify Fatal/Severe errors
2. Use read_idf_object to examine the problematic EnergyPlus object
3. Use edit_idf_object to fix the specific field (provide structured JSON parameters, not code)
4. Save a snapshot with save_idf_snapshot before major changes
5. Run the simulation again with run_simulation to verify

## STOP CONDITIONS — stop immediately and give a final answer if:
- check_simulation_result returns "dir_exists": true but all key files missing and "err_summary" is empty → simulation crashed before producing any output. Call run_simulation once to retry. If still no files, STOP.
- You have already called edit_idf_object 3+ times with the same field and value without success → STOP to avoid infinite loop.

## Common EnergyPlus Errors and Fixes
- "MaxHeatingTemp < MinHeatingTemp" → fix Maximum_Heating_Supply_Air_Temperature or Minimum_Heating_Supply_Air_Temperature
- "Zone has no surfaces" → check zone/surface name references
- "Schedule not found" → verify schedule name consistency
- "Construction not found" → verify construction/material name references
- "Outside boundary condition" → check surface boundary condition object names`)

	if intentSummary != "" {
		sb.WriteString("\n\n## Building Context\n")
		sb.WriteString(intentSummary)
	}

	sb.WriteString("\n\n## Available Tools\n")
	sb.WriteString(registry.GenerateToolDescriptions())

	return sb.String()
}
