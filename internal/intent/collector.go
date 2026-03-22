// 意图收集模块
// 通过 ReAct 模式与用户多轮对话，逐步收集建筑设计参数，填充 BuildingIntent 结构。
// 核心工具：ask_user（向用户提问）和 present_summary（展示汇总并请求确认）。
// 用户确认后返回 BuildingIntent，供 YAML 生成阶段使用。

package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/rag"
	"energyplus-agent/internal/react"
	"energyplus-agent/internal/skills"
	"energyplus-agent/internal/tools"
	"energyplus-agent/internal/ui"
)

// collectState 意图收集的内部状态（在工具回调间共享）
type collectState struct {
	confirmed    bool            // 用户已确认
	cancelled    bool            // 用户取消
	lastIntent   *BuildingIntent // 最近一次 LLM 提交的意图 JSON
	modification string          // 用户的修改意见（需要重新询问时填入）
}

// Collect 执行意图收集流程
// userDescription: 用户的初始建筑描述文字
// skillLoader: 已加载的 skill 集合（可为 nil）
// 返回: 确认后的 BuildingIntent
func Collect(
	ctx context.Context,
	llmClient *llm.Client,
	retriever rag.Retriever,
	skillLoader *skills.Loader,
	maxIter int,
	userDescription string,
) (*BuildingIntent, error) {
	slog.Info("[Intent] 开始意图收集", "user_input_len", len(userDescription))

	// RAG：检索相关的建筑示例（v0.1 为 Noop，不影响流程）
	docs, _ := retriever.Query(ctx, userDescription, 2)
	ragContext := rag.FormatDocs(docs)

	state := &collectState{}

	// 注册意图收集工具（含 skill 文件查询工具）
	registry := tools.NewRegistry()
	registerCollectTools(registry, state)
	if skillLoader != nil {
		registerSkillTools(registry, skillLoader)
	}

	// 构建 ReAct Agent
	agent := react.NewAgent(llmClient, registry, maxIter)

	// 构建 System Prompt（基础 + skill 指令）
	systemPrompt := SystemPromptIntentCollection
	if skillLoader != nil {
		systemPrompt += skillLoader.BuildPromptSection("intent")
	}

	// 构建用户消息（初始描述 + 可选 RAG 上下文）
	userMsg := userDescription
	if ragContext != "" {
		userMsg += "\n\n" + ragContext
	}

	// 循环执行，直到用户确认或取消
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			// 用户有修改意见，将其追加到用户消息
			slog.Info("[Intent] 用户有修改意见，重新收集", "attempt", attempt)
			userMsg = fmt.Sprintf("用户的修改意见：%s\n\n请根据修改意见更新意图，重新调用 present_summary 确认。", state.modification)
		}

		result, err := agent.Run(ctx, systemPrompt, userMsg)
		if err != nil {
			slog.Warn("[Intent] ReAct 运行出错", "err", err, "attempt", attempt)
			// 如果已经收集到了意图，仍可继续
			if state.lastIntent != nil {
				break
			}
			return nil, fmt.Errorf("意图收集失败: %w", err)
		}

		slog.Info("[Intent] ReAct 完成", "steps", len(result.Steps))

		if state.cancelled {
			return nil, fmt.Errorf("用户取消了操作")
		}

		if state.confirmed && state.lastIntent != nil {
			break
		}

		if state.lastIntent == nil {
			// LLM 没有调用 present_summary，用其最终回答作为提示
			slog.Warn("[Intent] LLM 未调用 present_summary", "final", result.FinalAnswer)
			// 强制要求 LLM 整理意图
			userMsg = "请立即调用 present_summary 工具，将目前收集到的信息整理成 JSON 格式展示给用户确认。"
			state.modification = ""
			continue
		}
	}

	if state.lastIntent == nil {
		return nil, fmt.Errorf("意图收集未完成，未获取到有效的建筑信息")
	}

	complete, missing := state.lastIntent.IsComplete()
	if !complete {
		slog.Warn("[Intent] 意图不完整", "missing", missing)
		// 继续，用不完整的意图生成 YAML（YAML 生成阶段会处理）
	}

	slog.Info("[Intent] 意图收集成功",
		"building", state.lastIntent.Building.Name,
		"city", state.lastIntent.Building.City,
		"zones", len(state.lastIntent.Geometry.Zones),
	)
	return state.lastIntent, nil
}

// registerCollectTools 注册意图收集阶段的工具
func registerCollectTools(registry *tools.Registry, state *collectState) {
	// ── 工具 1：ask_user ────────────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "ask_user",
				Description: "向用户提问以获取缺失的建筑参数信息。每次只问一个具体问题。",
				Parameters: tools.ObjectSchema(
					"提问参数",
					map[string]any{
						"question": tools.StringParam("要向用户提出的具体问题"),
					},
					[]string{"question"},
				),
			},
		},
		func(args map[string]any) (string, error) {
			question, err := tools.GetString(args, "question")
			if err != nil {
				return "", err
			}
			slog.Debug("[Intent] LLM 向用户提问", "question", question)
			answer, err := ui.AskQuestion(question)
			if err != nil {
				return "用户未回答", nil
			}
			if answer == "" {
				return "用户跳过了该问题（请使用默认值）", nil
			}
			slog.Debug("[Intent] 用户回答", "answer", answer)
			return answer, nil
		},
	)

	// ── 工具 2：present_summary ──────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "present_summary",
				Description: "将收集到的建筑意图整理为 JSON 格式，展示给用户确认。信息收集完毕后必须调用此工具。",
				Parameters: tools.ObjectSchema(
					"意图汇总参数",
					map[string]any{
						"intent_json": tools.StringParam("完整的 BuildingIntent JSON 字符串"),
					},
					[]string{"intent_json"},
				),
			},
		},
		func(args map[string]any) (string, error) {
			intentJSON, err := tools.GetString(args, "intent_json")
			if err != nil {
				return "", err
			}

			// 解析 JSON 为 BuildingIntent
			var intent BuildingIntent
			if err := json.Unmarshal([]byte(intentJSON), &intent); err != nil {
				return fmt.Sprintf("JSON 格式错误，请检查并重新提交: %v", err), nil
			}
			state.lastIntent = &intent

			// 展示意图摘要表格
			fields := intentToFields(&intent)
			logger.LLMThoughtEnd()
			fmt.Println() // 空行分隔
			ui.PrintIntentSummary(fields)

			// 请求用户确认
			confirmResult, modification := ui.ReadConfirm("以上信息是否正确？")

			switch confirmResult {
			case ui.ConfirmYes:
				state.confirmed = true
				slog.Info("[Intent] 用户确认意图")
				return "用户已确认，信息正确。可以开始生成 YAML 配置。", nil

			case ui.ConfirmModify:
				state.modification = modification
				slog.Info("[Intent] 用户要求修改", "modification", modification)
				return fmt.Sprintf("用户有修改意见：%s\n请根据意见修改相关字段，然后再次调用 present_summary。", modification), nil

			case ui.ConfirmCancel:
				state.cancelled = true
				slog.Info("[Intent] 用户取消")
				return "用户取消了操作", nil
			}

			return "已展示给用户", nil
		},
	)
}

// registerSkillTools 注册 skill 的文件查询工具：list_references、search_standard、read_reference
func registerSkillTools(registry *tools.Registry, loader *skills.Loader) {
	// 收集所有 intent 阶段 skill 的 references 目录
	var refDirs []string
	for _, s := range loader.ByPhase("intent") {
		if s.ReferencesDir != "" {
			refDirs = append(refDirs, s.ReferencesDir)
		}
	}
	defaultDir := ""
	if len(refDirs) > 0 {
		defaultDir = refDirs[0]
	}

	// ── 工具：list_references ──────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "list_references",
				Description: "列出规范参考文件目录中的所有表格文件名及其标题，用于了解有哪些可查阅的规范数据。",
				Parameters: tools.ObjectSchema(
					"list_references 参数",
					map[string]any{
						"dir": tools.StringParam("references 目录路径，留空则使用默认目录"),
					},
					[]string{},
				),
			},
		},
		func(args map[string]any) (string, error) {
			dir := tools.GetStringOr(args, "dir", defaultDir)
			if dir == "" {
				return "未配置 references 目录", nil
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return fmt.Sprintf("无法读取目录 %s: %v", dir, err), nil
			}
			var lines []string
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				// 读取文件第一个非空行作为标题
				title := readFirstHeading(filepath.Join(dir, e.Name()))
				lines = append(lines, fmt.Sprintf("- %s  →  %s", e.Name(), title))
			}
			if len(lines) == 0 {
				return "目录中没有找到 .md 文件", nil
			}
			return strings.Join(lines, "\n"), nil
		},
	)

	// ── 工具：search_standard ─────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "search_standard",
				Description: "在规范参考文件中搜索关键词，返回匹配的文件名和行内容。支持简单的子字符串匹配。",
				Parameters: tools.ObjectSchema(
					"search_standard 参数",
					map[string]any{
						"pattern": tools.StringParam("搜索关键词，例如 '严寒 外墙' 或 '办公 照明'"),
						"dir":     tools.StringParam("references 目录路径，留空则使用默认目录"),
					},
					[]string{"pattern"},
				),
			},
		},
		func(args map[string]any) (string, error) {
			pattern, err := tools.GetString(args, "pattern")
			if err != nil {
				return "", err
			}
			dir := tools.GetStringOr(args, "dir", defaultDir)
			if dir == "" {
				return "未配置 references 目录", nil
			}

			keywords := strings.Fields(strings.ToLower(pattern))
			var results []string

			entries, err := os.ReadDir(dir)
			if err != nil {
				return fmt.Sprintf("无法读取目录: %v", err), nil
			}

			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				fpath := filepath.Join(dir, e.Name())
				data, err := os.ReadFile(fpath)
				if err != nil {
					continue
				}
				lines := strings.Split(string(data), "\n")
				for i, line := range lines {
					lower := strings.ToLower(line)
					matched := true
					for _, kw := range keywords {
						if !strings.Contains(lower, kw) {
							matched = false
							break
						}
					}
					if matched && strings.TrimSpace(line) != "" {
						results = append(results, fmt.Sprintf("[%s:%d] %s", e.Name(), i+1, strings.TrimSpace(line)))
					}
				}
			}

			if len(results) == 0 {
				return fmt.Sprintf("未找到匹配 '%s' 的内容，可使用 list_references 查看可用文件", pattern), nil
			}
			return strings.Join(results, "\n"), nil
		},
	)

	// ── 工具：read_reference ──────────────────────────────────
	registry.Register(
		llm.Tool{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "read_reference",
				Description: "读取指定规范参考文件的完整内容，获取详细的数据表格。",
				Parameters: tools.ObjectSchema(
					"read_reference 参数",
					map[string]any{
						"filename": tools.StringParam("文件名（如 '围护结构传热系数基本要求.md'）或完整路径"),
					},
					[]string{"filename"},
				),
			},
		},
		func(args map[string]any) (string, error) {
			filename, err := tools.GetString(args, "filename")
			if err != nil {
				return "", err
			}
			// 支持仅文件名（自动拼接默认目录）或完整路径
			var fpath string
			if filepath.IsAbs(filename) || strings.Contains(filename, string(filepath.Separator)) || strings.Contains(filename, "/") {
				fpath = filename
			} else {
				fpath = filepath.Join(defaultDir, filename)
			}
			data, err := os.ReadFile(fpath)
			if err != nil {
				return fmt.Sprintf("无法读取文件 '%s': %v。请用 list_references 确认文件名", filename, err), nil
			}
			return string(data), nil
		},
	)
}

// readFirstHeading 读取 markdown 文件中第一个非空行作为标题描述
func readFirstHeading(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return strings.TrimLeft(line, "# ")
		}
	}
	return ""
}

// intentToFields 将 BuildingIntent 转换为展示字段 map
func intentToFields(b *BuildingIntent) map[string]string {
	f := func(v float64, unit string) string {
		if v == 0 {
			return ""
		}
		return fmt.Sprintf("%.2f %s", v, unit)
	}
	itoa := func(v int) string {
		if v == 0 {
			return ""
		}
		return fmt.Sprintf("%d", v)
	}

	fields := map[string]string{
		"建筑名称":       b.Building.Name,
		"建筑类型":       b.Building.Type,
		"城市":         b.Building.City,
		"纬度/经度":      fmt.Sprintf("%.2f / %.2f", b.Building.Latitude, b.Building.Longitude),
		"总面积(m²)":    f(b.Geometry.TotalArea, "m²"),
		"楼层数":        itoa(b.Geometry.NumFloors),
		"平面尺寸":       fmt.Sprintf("%.1f × %.1f m", b.Geometry.FloorWidth, b.Geometry.FloorDepth),
		"层高":         f(b.Geometry.FloorHeight, "m"),
		"热区数量":       itoa(len(b.Geometry.Zones)),
		"外墙 U 值":     f(b.Envelope.WallU, "W/m²K"),
		"屋顶 U 值":     f(b.Envelope.RoofU, "W/m²K"),
		"窗墙比 南/北/东/西": fmt.Sprintf("%.0f%%/%.0f%%/%.0f%%/%.0f%%",
			b.Window.WWRSouth*100, b.Window.WWRNorth*100,
			b.Window.WWREast*100, b.Window.WWRWest*100),
		"窗户 U 值/SHGC":  fmt.Sprintf("%.1f / %.2f", b.Window.UFactor, b.Window.SHGC),
		"供暖/制冷设定":     fmt.Sprintf("%.0f°C / %.0f°C", b.Schedule.HeatingSetpoint, b.Schedule.CoolingSetpoint),
		"使用类型":        b.Loads.OccupancyType,
		"工作日占用时间":     b.Schedule.WeekdayStart + " – " + b.Schedule.WeekdayEnd,
		"工作日空调时间":     b.Schedule.HVACWeekdayStart + " – " + b.Schedule.HVACWeekdayEnd,
		"仿真年份":        itoa(b.Simulation.Year),
	}
	return fields
}
