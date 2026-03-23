// Phase 6 Worker：执行单个参数变体的仿真
//
// 流程：复制基础 IDF → 应用 IDFEdit → 运行仿真（带 LLM 修复） → 提取指标
// Worker 不持有 SessionState（并发安全），通过参数传入所有依赖。

package paramanalysis

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/eplusrun"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/report"
	"energyplus-agent/internal/simulation"
)

// RunWorker 执行单个参数变体的完整仿真流程
// variation: 要执行的参数变体（含 IDFEdit 列表）
// baseIDFPath: 基础 IDF 文件路径（只读，Worker 复制后修改）
// workerBaseDir: Worker 工作目录根（各变体在此创建子目录）
// epwPath: EPW 气象文件路径
// runner, llmClient, cfg: 共享基础设施（goroutine 安全）
func RunWorker(
	ctx context.Context,
	variation ParamVariation,
	baseIDFPath, workerBaseDir, epwPath string,
	runner *eplusrun.Runner,
	llmClient *llm.Client,
	cfg *config.Config,
) WorkerResult {
	result := WorkerResult{
		Label:       variation.Label,
		Description: variation.Description,
	}

	slog.Info("[Worker] 开始", "label", variation.Label, "edits", len(variation.Edits))

	// ── Step 1: 创建变体工作目录 ──────────────────────────────────────
	varDir := filepath.Join(workerBaseDir, variation.Label)
	if err := os.MkdirAll(varDir, 0o755); err != nil {
		result.Error = fmt.Sprintf("创建工作目录失败: %v", err)
		slog.Warn("[Worker] 创建工作目录失败", "label", variation.Label, "err", err)
		return result
	}

	// ── Step 2: 复制基础 IDF ─────────────────────────────────────────
	varIDFPath := filepath.Join(varDir, variation.Label+".idf")
	if err := copyFile(baseIDFPath, varIDFPath); err != nil {
		result.Error = fmt.Sprintf("复制 IDF 失败: %v", err)
		slog.Warn("[Worker] 复制 IDF 失败", "label", variation.Label, "err", err)
		return result
	}

	// ── Step 3: 应用 IDFEdit ─────────────────────────────────────────
	for _, edit := range variation.Edits {
		slog.Debug("[Worker] 应用 IDFEdit",
			"label", variation.Label,
			"object_type", edit.ObjectType,
			"name", edit.Name,
			"field", edit.Field,
			"value", edit.Value,
		)
		if err := runner.EditIDF(ctx, varIDFPath, edit.ObjectType, edit.Name, edit.Field, edit.Value); err != nil {
			result.Error = fmt.Sprintf("应用 IDFEdit 失败 [%s.%s]: %v", edit.Name, edit.Field, err)
			slog.Warn("[Worker] IDFEdit 失败", "label", variation.Label, "edit", edit.Field, "err", err)
			return result
		}
	}

	slog.Info("[Worker] IDF 复制并编辑完成", "label", variation.Label, "idf", varIDFPath)

	// ── Step 4: 运行仿真（带 LLM 修复）─────────────────────────────
	maxFix := cfg.Modules.ParamAnalysis.MaxFixAttempts
	if maxFix <= 0 {
		maxFix = 3
	}
	simAgent := simulation.NewAgent(runner, llmClient, cfg, maxFix)

	simOutBase := filepath.Join(varDir, "sim")
	simResult, err := simAgent.RunWithRepair(
		ctx,
		varIDFPath,
		epwPath,
		simOutBase,
		"", // intentSummary — Worker 无 session，省略
		nil, // state — Worker 不持有 SessionState
	)
	if err != nil {
		result.Error = fmt.Sprintf("仿真运行异常: %v", err)
		slog.Warn("[Worker] 仿真异常", "label", variation.Label, "err", err)
		return result
	}

	result.FixAttempts = simResult.FixAttempts
	result.SimOutDir = simResult.SimOutDir
	result.TokensUsed = simResult.TotalTokens

	if !simResult.Success {
		result.Error = simResult.Error
		if result.Error == "" {
			result.Error = "仿真失败（未知原因）"
		}
		slog.Warn("[Worker] 仿真失败", "label", variation.Label, "fix_attempts", simResult.FixAttempts, "err", result.Error)
		return result
	}

	slog.Info("[Worker] 仿真完成",
		"label", variation.Label,
		"sim_out", simResult.SimOutDir,
		"fix_attempts", simResult.FixAttempts,
	)

	// ── Step 5: 读取仿真指标 ─────────────────────────────────────────
	if simResult.SimOutDir != "" {
		data, err := report.ReadSimData(simResult.SimOutDir)
		if err != nil {
			slog.Warn("[Worker] 读取仿真数据失败", "label", variation.Label, "err", err)
			// 指标读取失败不阻断——仿真本身成功，保留 success=true
		} else {
			result.Metrics = data.Summary
		}
	}

	result.Success = true
	return result
}

// copyFile 将 src 文件内容复制到 dst（dst 不存在则创建）
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("文件复制失败: %w", err)
	}
	return out.Sync()
}
