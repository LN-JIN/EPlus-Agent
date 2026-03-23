// YAML 生成模块
// 根据 BuildingIntent 调用 LLM（ReAct 模式）生成完整的 EnergyPlus YAML 配置文件。
// LLM 通过 write_yaml 工具提交生成内容，Go 侧负责写文件和基础语法验证。
// 若验证失败，Self-Healing 循环最多重试 maxHealIter 次，每次将错误反馈给 LLM 修复。

package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"energyplus-agent/internal/fault"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/react"
	"energyplus-agent/internal/tools"

	"gopkg.in/yaml.v3"
)

// generateState YAML 生成的内部状态
type generateState struct {
	yamlContent string // LLM 提交的 YAML 内容
	yamlPath    string // 写入的文件路径
	done        bool   // write_yaml 已被调用
}

// GenerateYAML 根据 BuildingIntent 生成 YAML 配置文件
// outputDir: 输出目录（文件名自动生成）
// sessionID: 用于写 ReAct 日志（空字符串则跳过）
// maxIter: ReAct 最大迭代次数
// maxHealIter: Self-Healing 最大修复次数
func GenerateYAML(
	ctx context.Context,
	llmClient *llm.Client,
	buildingIntent *BuildingIntent,
	outputDir string,
	sessionID string,
	maxIter int,
	maxHealIter int,
) (string, int, error) {
	slog.Info("[YAML] 开始生成 YAML 配置",
		"building", buildingIntent.Building.Name,
		"output_dir", outputDir,
	)

	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", 0, fmt.Errorf("创建输出目录失败 [%s]: %w", outputDir, err)
	}

	// 收集所有 ReAct 步骤（含 self-healing 轮次），函数退出时写日志
	var allSteps []logger.ReActStep
	if outputDir != "" && sessionID != "" {
		defer func() {
			if len(allSteps) == 0 {
				return
			}
			logPath := filepath.Join(outputDir, "react_logs", sessionID+"_yaml_generate.md")
			if writeErr := logger.WriteReActLog(logPath, "yaml_generate", sessionID, allSteps); writeErr != nil {
				slog.Warn("[YAML] ReAct 日志写入失败", "err", writeErr)
			}
		}()
	}

	totalTokens := 0
	appendSteps := func(r *react.Result) {
		if r == nil {
			return
		}
		totalTokens += r.TotalTokens
		for _, s := range r.Steps {
			allSteps = append(allSteps, logger.ReActStep{
				Iter:        s.Iter,
				Thought:     s.Thought,
				Action:      s.Action,
				ActionInput: s.ActionInput,
				Observation: s.Observation,
				IsFinal:     s.IsFinal,
				FinalAnswer: s.FinalAnswer,
			})
		}
	}

	// 将 intent 序列化为 JSON 作为用户消息
	intentJSON, err := json.MarshalIndent(buildingIntent, "", "  ")
	if err != nil {
		return "", 0, fmt.Errorf("序列化 BuildingIntent 失败: %w", err)
	}

	state := &generateState{}
	registry := tools.NewRegistry()
	registerGenerateTools(registry, state)

	agent := react.NewAgent(llmClient, registry, maxIter)

	userMsg := fmt.Sprintf(`请根据以下建筑意图，生成完整的 EnergyPlus YAML 配置文件，然后调用 write_yaml 工具提交。

## 建筑意图 (BuildingIntent)
%s

## 要求
1. 生成完整 YAML，包含所有必要节点（SimulationControl 到 Schedule）
2. 几何尺寸根据总面积和楼层数推导
3. 材料参数根据 U 值反推（选用合理的多层构造）
4. 所有名称引用必须一致（Construction → Material，Surface → Zone 等）
5. 完成后调用 write_yaml 工具提交完整内容
`, string(intentJSON))

	logger.Phase("YAML 生成", "LLM 正在根据建筑意图生成配置...")

	// 第一次生成
	r0, err := agent.Run(ctx, BuildSystemPrompt(SystemPromptYAMLGeneration, registry), userMsg)
	appendSteps(r0)
	if err != nil && !state.done {
		return "", totalTokens, fmt.Errorf("YAML 生成失败: %w", err)
	}

	if !state.done || state.yamlContent == "" {
		return "", totalTokens, fmt.Errorf("LLM 未调用 write_yaml 工具，生成失败")
	}

	// 写入文件
	yamlPath, err := writeYAMLFile(state.yamlContent, outputDir, buildingIntent.Building.Name)
	if err != nil {
		return "", totalTokens, err
	}
	state.yamlPath = yamlPath
	slog.Info("[YAML] 文件已写入", "path", yamlPath)

	// Self-Healing 循环：YAML 语法验证 + 修复
	guard := &fault.SameErrorGuard{MaxRepeat: 2}
	for healIter := 0; healIter < maxHealIter; healIter++ {
		validErr := validateYAMLSyntax(state.yamlContent)
		if validErr == nil {
			slog.Info("[YAML] 语法验证通过", "path", yamlPath)
			break
		}

		if spinning, hint := guard.Observe(validErr.Error()); spinning {
			slog.Warn("[YAML] 检测到修复空转，终止自愈循环", "hint", hint)
			break
		}

		slog.Warn("[YAML] 语法验证失败，尝试自修复",
			"heal_iter", healIter+1,
			"error", validErr,
		)
		logger.Phase("YAML 自修复", fmt.Sprintf("第 %d 次修复...", healIter+1))

		// 让 LLM 修复错误
		fixMsg := fmt.Sprintf(`上一次生成的 YAML 存在语法错误，请修复后重新调用 write_yaml 提交。

## 错误信息
%v

## 原始 YAML（有问题的版本）
%s

请修复语法错误，确保 YAML 格式正确，然后再次调用 write_yaml。
`, validErr, state.yamlContent)

		state.done = false
		state.yamlContent = ""

		rHeal, healErr := agent.Run(ctx, BuildSystemPrompt(SystemPromptYAMLGeneration, registry), fixMsg)
		appendSteps(rHeal)
		if healErr != nil && !state.done {
			slog.Warn("[YAML] 自修复 ReAct 失败", "err", healErr)
			break
		}

		if state.done && state.yamlContent != "" {
			// 更新文件
			yamlPath, err = writeYAMLFile(state.yamlContent, outputDir, buildingIntent.Building.Name)
			if err != nil {
				return "", totalTokens, err
			}
			state.yamlPath = yamlPath
		}
	}

	slog.Info("[YAML] 生成流程完成", "path", state.yamlPath)
	return state.yamlPath, totalTokens, nil
}

// registerGenerateTools 注册 YAML 生成阶段的工具
func registerGenerateTools(registry *tools.Registry, state *generateState) {
	// ── 工具 1：write_yaml ──────────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "write_yaml",
				Description: "将生成的完整 YAML 内容提交。只调用一次，传入完整的 YAML 字符串（非 JSON）。",
				Parameters: tools.ObjectSchema(
					"YAML 提交参数",
					map[string]any{
						"content": tools.StringParam("完整的 EnergyPlus YAML 配置文件内容字符串"),
					},
					[]string{"content"},
				),
			},
		},
		func(args map[string]any) (string, error) {
			content, err := tools.GetString(args, "content")
			if err != nil {
				return "", err
			}
			if len(content) < 100 {
				return "ERROR: YAML 内容过短，看起来不完整，请提交完整的配置文件", nil
			}

			state.yamlContent = content
			state.done = true

			slog.Info("[YAML] write_yaml 工具被调用", "content_len", len(content))
			return fmt.Sprintf("YAML 内容已接收（%d 字节）。流程将继续验证。", len(content)), nil
		},
	)

	// ── 工具 2：validate_section ────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "validate_section",
				Description: "验证某个 YAML 片段的语法正确性（可选，用于自检某个节点）。",
				Parameters: tools.ObjectSchema(
					"验证参数",
					map[string]any{
						"section_name": tools.StringParam("节点名称（如 Material、Construction 等）"),
						"yaml_text":    tools.StringParam("要验证的 YAML 片段"),
					},
					[]string{"section_name", "yaml_text"},
				),
			},
		},
		func(args map[string]any) (string, error) {
			sectionName := tools.GetStringOr(args, "section_name", "unknown")
			yamlText := tools.GetStringOr(args, "yaml_text", "")

			if yamlText == "" {
				return "ERROR: yaml_text 为空", nil
			}

			var out any
			if err := yaml.Unmarshal([]byte(yamlText), &out); err != nil {
				slog.Debug("[YAML] 片段验证失败", "section", sectionName, "err", err)
				return fmt.Sprintf("YAML 语法错误 [%s]: %v", sectionName, err), nil
			}

			slog.Debug("[YAML] 片段验证通过", "section", sectionName)
			return fmt.Sprintf("[%s] YAML 语法正确", sectionName), nil
		},
	)
}

// writeYAMLFile 将 YAML 内容写入文件，返回文件路径
func writeYAMLFile(content, outputDir, buildingName string) (string, error) {
	// 清理文件名中的非法字符
	safeName := sanitizeFilename(buildingName)
	if safeName == "" {
		safeName = "building"
	}
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.yaml", safeName, timestamp)
	filePath := filepath.Join(outputDir, filename)

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入 YAML 文件失败 [%s]: %w", filePath, err)
	}
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return filePath, nil
	}
	return absPath, nil
}

// validateYAMLSyntax 验证 YAML 字符串的语法正确性
func validateYAMLSyntax(content string) error {
	var out any
	if err := yaml.Unmarshal([]byte(content), &out); err != nil {
		return fmt.Errorf("YAML 语法错误: %w", err)
	}
	if out == nil {
		return fmt.Errorf("YAML 内容为空")
	}
	return nil
}

// sanitizeFilename 清理字符串，使其可作为文件名
func sanitizeFilename(name string) string {
	result := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			result = append(result, r)
		case r == '-' || r == '_':
			result = append(result, r)
		case r == ' ':
			result = append(result, '_')
		}
	}
	if len(result) > 50 {
		result = result[:50]
	}
	return string(result)
}
