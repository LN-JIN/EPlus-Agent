// PhaseModule 包装器
// 将 intent.Collect 和 intent.GenerateYAML 封装为 session.PhaseModule 接口，
// 供 Orchestrator 统一驱动，而无需直接依赖 intent 包的内部细节。

package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/rag"
	"energyplus-agent/internal/session"
	"energyplus-agent/internal/skills"
)

// ── Phase 1: 意图收集 ────────────────────────────────────────────────────────

// CollectModule 封装意图收集（Phase 1）
type CollectModule struct {
	llmClient   *llm.Client
	retriever   rag.Retriever
	skillLoader *skills.Loader
	cfg         *config.Config
}

// NewCollectModule 创建意图收集模块
func NewCollectModule(
	llmClient *llm.Client,
	retriever rag.Retriever,
	skillLoader *skills.Loader,
	cfg *config.Config,
) *CollectModule {
	return &CollectModule{
		llmClient:   llmClient,
		retriever:   retriever,
		skillLoader: skillLoader,
		cfg:         cfg,
	}
}

func (m *CollectModule) Name() string { return string(session.PhaseIntentCollection) }

func (m *CollectModule) Run(ctx context.Context, state *session.SessionState) error {
	buildingIntent, tokens, err := Collect(
		ctx,
		m.llmClient,
		m.retriever,
		m.skillLoader,
		m.cfg.Modules.Intent.MaxReactIter,
		state.UserInput,
		state.SessionID,
		m.cfg.Session.OutputDir,
	)
	state.AddTokens(tokens)
	if err != nil {
		return err
	}

	// 序列化为 JSON 供 Phase 2 使用
	data, err := json.Marshal(buildingIntent)
	if err != nil {
		return fmt.Errorf("序列化 BuildingIntent 失败: %w", err)
	}
	state.IntentJSON = string(data)

	// 传递输出变量
	state.OutputVariables = buildingIntent.OutputVariables

	// 生成自然语言摘要（注入后续阶段 SystemPrompt）
	state.IntentSummary = buildIntentSummary(buildingIntent)

	slog.Info("[IntentModule] Phase 1 完成",
		"building", buildingIntent.Building.Name,
		"city", buildingIntent.Building.City,
	)
	return nil
}

// buildIntentSummary 将 BuildingIntent 转换为自然语言摘要文字
// 此摘要注入 Phase 3-6 的 SystemPrompt，让各 Agent 了解建筑背景
func buildIntentSummary(b *BuildingIntent) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("建筑名称: %s | 类型: %s | 城市: %s\n",
		b.Building.Name, b.Building.Type, b.Building.City))
	sb.WriteString(fmt.Sprintf("层数: %d 层 | 总面积: %.0f m² | 层高: %.1f m\n",
		b.Geometry.NumFloors, b.Geometry.TotalArea, b.Geometry.FloorHeight))
	sb.WriteString(fmt.Sprintf("外墙 U=%.2f W/m²K | 屋顶 U=%.2f W/m²K | 窗户 U=%.2f W/m²K, SHGC=%.2f\n",
		b.Envelope.WallU, b.Envelope.RoofU, b.Window.UFactor, b.Window.SHGC))
	sb.WriteString(fmt.Sprintf("空调供暖设定点: %.0f°C | 制冷设定点: %.0f°C\n",
		b.Schedule.HeatingSetpoint, b.Schedule.CoolingSetpoint))
	if len(b.OutputVariables) > 0 {
		sb.WriteString(fmt.Sprintf("输出变量需求: %s\n", strings.Join(b.OutputVariables, ", ")))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ── Phase 2: YAML 生成 ───────────────────────────────────────────────────────

// GenerateModule 封装 YAML 生成（Phase 2）
type GenerateModule struct {
	llmClient *llm.Client
	cfg       *config.Config
}

// NewGenerateModule 创建 YAML 生成模块
func NewGenerateModule(llmClient *llm.Client, cfg *config.Config) *GenerateModule {
	return &GenerateModule{llmClient: llmClient, cfg: cfg}
}

func (m *GenerateModule) Name() string { return string(session.PhaseYAMLGenerating) }

func (m *GenerateModule) Run(ctx context.Context, state *session.SessionState) error {
	// 从 session 反序列化 BuildingIntent
	if state.IntentJSON == "" {
		return fmt.Errorf("IntentJSON 为空，Phase 1 必须先完成")
	}
	var buildingIntent BuildingIntent
	if err := json.Unmarshal([]byte(state.IntentJSON), &buildingIntent); err != nil {
		return fmt.Errorf("反序列化 BuildingIntent 失败: %w", err)
	}

	yamlPath, tokens, err := GenerateYAML(
		ctx,
		m.llmClient,
		&buildingIntent,
		m.cfg.Session.OutputDir,
		state.SessionID,
		m.cfg.Modules.YAMLGenerate.MaxReactIter,
		m.cfg.Modules.YAMLGenerate.MaxHealAttempts,
	)
	state.AddTokens(tokens)
	if err != nil {
		return err
	}

	state.YAMLPath = yamlPath
	state.AddSnapshot("initial_yaml", yamlPath)

	slog.Info("[GenerateModule] Phase 2 完成", "yaml_path", yamlPath)
	return nil
}
