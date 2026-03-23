// Phase 3: YAML→IDF 转换模块（带自愈循环）
// 调用 EPlus-MCP CLI 将 YAML 配置转换为 EnergyPlus IDF 文件。
// 转换失败时，LLM 分析错误信息，修复 YAML，然后重试（程序化修复循环，非 ReAct）。
// 每次修复后保存新版 YAML 快照，便于回溯。
//
// 选择程序化修复循环（非 ReAct）的原因：
// YAML→IDF 转换失败通常是 Schema 校验错误，原因明确，无需多步推理；
// 一次 LLM 调用即可根据错误生成修复版 YAML，再重试即可。

package idfconvert

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/eplusrun"
	"energyplus-agent/internal/fault"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/session"
	"energyplus-agent/internal/ui"
)

// Module Phase 3 YAML→IDF 转换模块
type Module struct {
	runner    *eplusrun.Runner
	llmClient *llm.Client
	cfg       *config.Config
}

// New 创建 Phase 3 模块
func New(runner *eplusrun.Runner, llmClient *llm.Client, cfg *config.Config) *Module {
	return &Module{
		runner:    runner,
		llmClient: llmClient,
		cfg:       cfg,
	}
}

func (m *Module) Name() string { return string(session.PhaseIDFConverting) }

// Run 执行 YAML→IDF 转换，带自愈循环
func (m *Module) Run(ctx context.Context, state *session.SessionState) error {
	logger.Phase("阶段 3/6", "YAML→IDF 转换 — 将配置转换为 EnergyPlus IDF 文件")

	if state.YAMLPath == "" {
		return fmt.Errorf("YAMLPath 为空，Phase 2 必须先完成")
	}

	maxAttempts := m.cfg.Modules.IDFConvert.MaxHealAttempts
	if maxAttempts <= 0 {
		maxAttempts = 8
	}

	idfOutputDir := filepath.Join(m.cfg.Session.OutputDir, "idf",
		strings.TrimSuffix(filepath.Base(state.YAMLPath), filepath.Ext(state.YAMLPath)))

	currentYAML := state.YAMLPath
	var lastErr error
	guard := &fault.SameErrorGuard{MaxRepeat: 2}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			slog.Info("[IDF Convert] 自愈重试", "attempt", attempt, "max", maxAttempts)
			ui.PrintInfo(fmt.Sprintf("YAML→IDF 修复重试 (%d/%d)...", attempt, maxAttempts))
		}

		idfPath, err := m.runner.ConvertYAMLToIDF(ctx, currentYAML, idfOutputDir)
		if err == nil {
			// 成功
			state.IDFPath = idfPath
			state.AddIDFSnapshot(fmt.Sprintf("idf_v%d", attempt), idfPath, "")
			ui.PrintSuccess(fmt.Sprintf("IDF 文件已生成: %s", idfPath))
			slog.Info("[IDF Convert] 转换成功", "idf_path", idfPath, "attempts", attempt)
			return nil
		}

		lastErr = err
		slog.Warn("[IDF Convert] 转换失败，尝试 LLM 修复", "err", err, "attempt", attempt)

		if fault.IsFatal(err) {
			slog.Error("[IDF Convert] 检测到环境致命错误，终止重试", "err", err)
			ui.PrintError("检测到环境配置问题（如 Python 未安装），修复 YAML 无法解决此类错误。\n请检查 Python 环境或在 config.yaml 中设置 session.python_path。")
			return fmt.Errorf("环境配置错误（无法自动修复）: %w", err)
		}

		if spinning, hint := guard.Observe(err.Error()); spinning {
			slog.Warn("[IDF Convert] 检测到修复空转，终止重试", "hint", hint)
			ui.PrintWarning(hint)
			break
		}

		if attempt == maxAttempts {
			break
		}

		// LLM 修复：读取 YAML 内容 + 错误信息 → 生成修复版 YAML
		fixedYAML, healTokens, fixErr := m.healWithLLM(ctx, state, currentYAML, err.Error())
		state.AddTokens(healTokens)
		if fixErr != nil {
			slog.Warn("[IDF Convert] LLM 修复失败", "err", fixErr)
			break
		}

		// 写入修复版 YAML（带版本号）
		stem := strings.TrimSuffix(filepath.Base(state.YAMLPath), filepath.Ext(state.YAMLPath))
		fixedPath := filepath.Join(m.cfg.Session.OutputDir, "yaml",
			stem, fmt.Sprintf("v%d_healed.yaml", attempt))
		if writeErr := writeYAMLFile(fixedPath, fixedYAML); writeErr != nil {
			slog.Warn("[IDF Convert] 写入修复 YAML 失败", "err", writeErr)
			break
		}

		state.AddSnapshot(fmt.Sprintf("yaml_healed_v%d", attempt), fixedPath)
		currentYAML = fixedPath
	}

	return fmt.Errorf("YAML→IDF 转换失败（已尝试 %d 次）: %w", maxAttempts, lastErr)
}

// healWithLLM 调用 LLM 一次，分析转换错误并返回修复后的 YAML 内容及 token 消耗
func (m *Module) healWithLLM(ctx context.Context, state *session.SessionState, yamlPath, errMsg string) (string, int, error) {
	// 读取当前 YAML 内容
	yamlContent, err := os.ReadFile(yamlPath)
	if err != nil {
		return "", 0, fmt.Errorf("读取 YAML 文件失败: %w", err)
	}

	sysPrompt := `You are an EnergyPlus YAML configuration expert. Your task is to fix a YAML configuration file that failed to convert to IDF format.

Analyze the error message, identify the problematic field(s) in the YAML, and return the complete corrected YAML.

IMPORTANT:
- Return ONLY the corrected YAML content, no explanations, no markdown fences
- Keep all unchanged sections identical to the original
- Fix only the fields related to the error`

	if state.IntentSummary != "" {
		sysPrompt += "\n\n## Building Context\n" + state.IntentSummary
	}

	userMsg := fmt.Sprintf("## Conversion Error\n%s\n\n## Current YAML\n```yaml\n%s\n```\n\nReturn the corrected YAML:",
		errMsg, string(yamlContent))

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: sysPrompt},
		{Role: llm.RoleUser, Content: userMsg},
	}

	var fixedYAML strings.Builder
	var healTokens int
	_, err = m.llmClient.ChatStream(ctx, messages, nil,
		func(chunk string) { fixedYAML.WriteString(chunk) },
		func(u llm.Usage) { healTokens = u.TotalTokens },
	)
	if err != nil {
		return "", healTokens, fmt.Errorf("LLM 修复调用失败: %w", err)
	}

	result := strings.TrimSpace(fixedYAML.String())
	// 去除 LLM 可能返回的 markdown 代码块包装
	result = stripMarkdownFence(result)

	if result == "" {
		return "", healTokens, fmt.Errorf("LLM 返回空内容")
	}

	return result, healTokens, nil
}

// stripMarkdownFence 去除 ```yaml ... ``` 包装（LLM 有时会加上）
func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// 找到第一行（fence行），从第二行开始
		idx := strings.Index(s, "\n")
		if idx >= 0 {
			s = s[idx+1:]
		}
		// 去掉末尾的 ```
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

// writeYAMLFile 将 YAML 内容写入指定路径（自动创建目录）
func writeYAMLFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败 [%s]: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

