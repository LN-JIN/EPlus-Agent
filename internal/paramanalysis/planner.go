// Phase 6 Planner：ReAct Agent，生成参数变体方案列表
//
// Planner 通过两个工具与 LLM 交互：
//   - list_idf_objects(object_type): 读取 IDF 中指定类型对象的名称和字段值
//   - submit_variations(variations): LLM 提交 JSON 字符串形式的变体方案列表
//
// 架构约束：Planner 只输出 []ParamVariation 数据，不感知 Worker 的存在。

package paramanalysis

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"context"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/eplusrun"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/react"
	"energyplus-agent/internal/tools"
)

// Planner 负责通过 ReAct 循环生成参数变体方案
type Planner struct {
	llmClient *llm.Client
	runner    *eplusrun.Runner
	cfg       *config.Config
}

// NewPlanner 创建 Planner
func NewPlanner(llmClient *llm.Client, runner *eplusrun.Runner, cfg *config.Config) *Planner {
	return &Planner{
		llmClient: llmClient,
		runner:    runner,
		cfg:       cfg,
	}
}

// Plan 运行 Planner ReAct 循环，生成参数变体方案列表
// analysisGoal: 用户的自然语言分析目标
// intentSummary: Phase 1 生成的建筑意图摘要
// baseIDFPath: 基础 IDF 文件路径
// baselineReport: Phase 5 能耗报告全文（空字符串表示无）
// sessionID: 用于 ReAct 日志命名
func (p *Planner) Plan(
	ctx context.Context,
	analysisGoal, intentSummary, baseIDFPath, baselineReport, sessionID string,
) ([]ParamVariation, int, error) {
	slog.Info("[Planner] 开始规划", "goal", analysisGoal, "idf", baseIDFPath)

	// ── 构建工具注册表 ──────────────────────────────────────────────────
	registry := tools.NewRegistry()

	// 工具 1: list_idf_objects — 读取 IDF 对象字段
	registry.Register(llm.Tool{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "list_idf_objects",
			Description: "Read all objects of a given type from the base IDF file, returning their names and field values. Call this before generating variations to discover real object names and current field values.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"object_type": tools.StringParam("EnergyPlus object type, e.g. Material, WindowMaterial:SimpleGlazingSystem, Lights, ElectricEquipment"),
				},
				"required": []string{"object_type"},
			},
		},
	}, func(args map[string]any) (string, error) {
		objectType, err := tools.GetString(args, "object_type")
		if err != nil {
			return "", err
		}
		result, err := p.runner.ReadIDFObjects(ctx, baseIDFPath, objectType)
		if err != nil {
			return fmt.Sprintf("ERROR: %v", err), nil
		}
		return result, nil
	})

	// 工具 2: submit_variations — LLM 提交变体方案
	var captured []ParamVariation
	registry.Register(llm.Tool{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "submit_variations",
			Description: "Submit the parametric variation plans as a JSON array string. Each variation must have label (slug), description (Chinese), and edits (array of IDFEdit objects). Must include a 'baseline' variation with empty edits.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"variations": tools.StringParam("JSON array string of ParamVariation objects"),
				},
				"required": []string{"variations"},
			},
		},
	}, func(args map[string]any) (string, error) {
		variationsStr, err := tools.GetString(args, "variations")
		if err != nil {
			return "", err
		}
		var variations []ParamVariation
		if err := json.Unmarshal([]byte(variationsStr), &variations); err != nil {
			return fmt.Sprintf("ERROR: 解析 JSON 失败: %v\n请确保提交的是合法 JSON 数组字符串。", err), nil
		}
		if len(variations) == 0 {
			return "ERROR: 变体列表为空，请至少提交 baseline + 1 个参数变体", nil
		}
		captured = variations
		return fmt.Sprintf("OK: 已接收 %d 个变体方案", len(variations)), nil
	})

	// ── 构建 System Prompt ──────────────────────────────────────────────
	systemPrompt := p.buildSystemPrompt(registry, intentSummary, baseIDFPath, baselineReport)

	// ── 构建用户消息 ───────────────────────────────────────────────────
	userMsg := fmt.Sprintf("## 参数分析目标\n%s\n\n请先调用 list_idf_objects 探索 IDF 中的对象，然后调用 submit_variations 提交变体方案。", analysisGoal)

	// ── 运行 ReAct Agent ───────────────────────────────────────────────
	maxIter := p.cfg.Modules.ParamAnalysis.MaxReactIter
	if maxIter <= 0 {
		maxIter = 12
	}
	agent := react.NewAgent(p.llmClient, registry, maxIter)
	result, err := agent.Run(ctx, systemPrompt, userMsg)

	plannerTokens := 0
	if result != nil {
		plannerTokens = result.TotalTokens
	}

	// 写入 ReAct 日志
	if result != nil && len(result.Steps) > 0 && p.cfg.Session.OutputDir != "" {
		logPath := filepath.Join(p.cfg.Session.OutputDir, "react_logs",
			sessionID+"_paramanalysis_planner.md")
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
		if writeErr := logger.WriteReActLog(logPath, "paramanalysis_planner", sessionID, steps); writeErr != nil {
			slog.Warn("[Planner] ReAct 日志写入失败", "err", writeErr)
		}
	}

	if err != nil {
		// ReAct 超限但已捕获变体则继续
		if len(captured) == 0 {
			return nil, plannerTokens, fmt.Errorf("Planner ReAct 失败且未收到变体方案: %w", err)
		}
		slog.Warn("[Planner] ReAct 超限，使用已捕获的变体", "variations", len(captured))
	}

	if len(captured) == 0 {
		return nil, plannerTokens, fmt.Errorf("Planner 未调用 submit_variations，无变体方案")
	}

	slog.Info("[Planner] 规划完成", "variations", len(captured), "react_iters", len(result.Steps))
	return captured, plannerTokens, nil
}

// buildSystemPrompt 构建 Planner 的 System Prompt
func (p *Planner) buildSystemPrompt(registry *tools.Registry, intentSummary, baseIDFPath, baselineReport string) string {
	var sb strings.Builder

	sb.WriteString(`You are a building energy parametric study expert. Your task is to design parametric variations for an EnergyPlus IDF file based on the user's analysis goal.

## Workflow
1. Call list_idf_objects to discover the real names and current values of relevant objects in the IDF
2. Based on what you find and the analysis goal, design 3-6 parametric variations
3. Call submit_variations to submit your variation plans as a JSON array string

## IDFEdit Format
Each edit in a variation must use this exact structure:
- object_type: EnergyPlus object class name (uppercase when stored in IDF, e.g. "Material")
- name: The exact Name field value of the object (get this from list_idf_objects!)
- field: The eppy attribute name (case-sensitive, e.g. "Thickness", "UFactor")
- value: The new value as a string (eppy auto-converts types)

## Requirements
- Always include a "baseline" variation with empty edits []
- Label must be a slug (lowercase, underscores, no spaces), e.g. "wall_ins_5cm"
- Description must be in Chinese
- Generate 3-6 total variations (including baseline)
- Values must be physically reasonable (e.g., thickness in meters: 0.03 not 3)

## Supported Parametric Study Types (stable, single-field edits)
Only generate variations for these supported types:
- Wall/roof insulation thickness: object_type="Material", field="Thickness" (meters)
- Glazing U-factor: object_type="WindowMaterial:SimpleGlazingSystem", field="UFactor" (W/m2-K)
- Glazing SHGC: object_type="WindowMaterial:SimpleGlazingSystem", field="Solar_Heat_Gain_Coefficient" (0-1)
- Lighting power density: object_type="Lights", field="Watts_per_Zone_Floor_Area" (W/m2)
- Equipment power density: object_type="ElectricEquipment", field="Watts_per_Zone_Floor_Area" (W/m2)

DO NOT generate variations for: window-to-wall ratio (requires geometry changes), HVAC system type or COP (requires system replacement), temperature setpoints (Schedule:Compact is fragile).

## submit_variations Format
Call submit_variations with a JSON string like:
[{"label":"baseline","description":"基线（原始 IDF）","edits":[]},{"label":"wall_ins_5cm","description":"外墙保温 5cm","edits":[{"object_type":"Material","name":"ExWall_Ins","field":"Thickness","value":"0.05"}]}]
`)

	sb.WriteString("\n## Base IDF File\n")
	sb.WriteString(baseIDFPath)

	if intentSummary != "" {
		sb.WriteString("\n\n## Building Intent Summary\n")
		sb.WriteString(intentSummary)
	}

	if baselineReport != "" {
		// 截断避免超出 context
		reportExcerpt := baselineReport
		if len(reportExcerpt) > 3000 {
			reportExcerpt = reportExcerpt[:3000] + "\n...(truncated)"
		}
		sb.WriteString("\n\n## Phase 5 Baseline Energy Report (for reference)\n")
		sb.WriteString(reportExcerpt)
	}

	sb.WriteString("\n\n## Available Tools\n")
	sb.WriteString(registry.GenerateToolDescriptions())

	return sb.String()
}
