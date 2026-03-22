// Orchestrator 主流程编排模块
// 实现 v0.1 的线性流程：意图收集 → YAML 生成 → MCP 转换验证 → 展示 IDF。
// 持有所有基础设施对象（LLM 客户端、MCP 客户端），协调各阶段的执行。
// 每个阶段完成后更新 SessionState，方便进度追踪和问题定位。

package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/intent"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/mcp"
	"energyplus-agent/internal/rag"
	"energyplus-agent/internal/skills"
	"energyplus-agent/internal/ui"
)

// Orchestrator 主流程协调器
type Orchestrator struct {
	cfg         *config.Config
	llmClient   *llm.Client
	mcpClient   *mcp.Client
	retriever   rag.Retriever
	skillLoader *skills.Loader
	state       *SessionState
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

	sessionID := fmt.Sprintf("session_%s", time.Now().Format("20060102_150405"))

	skillLoader := skills.Load("skills")

	return &Orchestrator{
		cfg:         cfg,
		llmClient:   llmClient,
		mcpClient:   mcpClient,
		retriever:   &rag.NoopRetriever{},
		skillLoader: skillLoader,
		state:       NewSessionState(sessionID),
	}
}

// Run 执行完整的 v0.1 流程
// userInput: 用户的初始建筑描述
func (o *Orchestrator) Run(ctx context.Context, userInput string) error {
	slog.Info("[Orch] 会话开始", "session_id", o.state.SessionID)

	// ══════════════════════════════════════════════════════
	// Step 0: 初始化 MCP Server 连接
	// ══════════════════════════════════════════════════════
	logger.Phase("初始化", "连接 MCP Server...")
	mcpCtx, mcpCancel := context.WithTimeout(ctx, time.Duration(o.cfg.MCP.InitTimeoutSec)*time.Second)
	defer mcpCancel()

	if err := o.mcpClient.Initialize(mcpCtx); err != nil {
		ui.PrintWarning("MCP Server 连接失败，将跳过配置验证步骤")
		slog.Warn("[Orch] MCP 初始化失败（继续运行）", "err", err)
	} else {
		ui.PrintSuccess("MCP Server 连接成功")
		// 清空旧状态，避免上次会话遗留数据干扰
		_ = o.mcpClient.ClearAll(ctx)
	}

	// ══════════════════════════════════════════════════════
	// Step 1: 意图收集
	// ══════════════════════════════════════════════════════
	logger.Phase("阶段 1/4", "意图收集 — 与您沟通，了解建筑设计参数")
	o.state.Phase = PhaseIntentCollection

	buildingIntent, err := intent.Collect(
		ctx,
		o.llmClient,
		o.retriever,
		o.skillLoader,
		o.cfg.Session.MaxReactIter,
		userInput,
	)
	if err != nil {
		o.state.Phase = PhaseCancelled
		if strings.Contains(err.Error(), "取消") {
			ui.PrintWarning("用户取消了操作")
			return nil
		}
		o.state.Phase = PhaseFailed
		o.state.FailureReason = err.Error()
		return fmt.Errorf("意图收集失败: %w", err)
	}

	o.state.Intent = buildingIntent
	o.state.IntentDoneAt = time.Now()
	slog.Info("[Orch] 意图收集完成",
		"building", buildingIntent.Building.Name,
		"duration", time.Since(o.state.CreatedAt).Round(time.Second),
	)

	// ══════════════════════════════════════════════════════
	// Step 2: YAML 生成
	// ══════════════════════════════════════════════════════
	logger.Phase("阶段 2/4", "YAML 生成 — LLM 根据意图构建 EnergyPlus 配置")
	o.state.Phase = PhaseYAMLGenerating

	yamlPath, err := intent.GenerateYAML(
		ctx,
		o.llmClient,
		buildingIntent,
		o.cfg.Session.OutputDir,
		o.cfg.Session.MaxReactIter,
		o.cfg.Session.MaxHealIter,
	)
	if err != nil {
		o.state.Phase = PhaseFailed
		o.state.FailureReason = err.Error()
		return fmt.Errorf("YAML 生成失败: %w", err)
	}

	o.state.YAMLPath = yamlPath
	o.state.YAMLDoneAt = time.Now()
	o.state.AddSnapshot("initial_yaml", yamlPath)

	ui.PrintSuccess(fmt.Sprintf("YAML 文件已生成: %s", yamlPath))
	slog.Info("[Orch] YAML 生成完成",
		"path", yamlPath,
		"duration", time.Since(o.state.IntentDoneAt).Round(time.Second),
	)

	// ══════════════════════════════════════════════════════
	// Step 3: MCP 转换验证
	// ══════════════════════════════════════════════════════
	logger.Phase("阶段 3/4", "MCP 验证 — 加载并验证配置完整性")
	o.state.Phase = PhaseIDFConverting

	validationSummary, err := o.runMCPValidation(ctx, yamlPath)
	if err != nil {
		// MCP 验证失败不阻断流程（仅警告）
		ui.PrintWarning(fmt.Sprintf("MCP 验证遇到问题: %v", err))
		slog.Warn("[Orch] MCP 验证失败（继续展示）", "err", err)
		validationSummary = fmt.Sprintf("验证未完成: %v", err)
	} else {
		ui.PrintSuccess("配置验证通过")
	}

	o.state.ValidationSummary = validationSummary
	o.state.ConvertDoneAt = time.Now()
	slog.Info("[Orch] MCP 验证完成",
		"duration", time.Since(o.state.YAMLDoneAt).Round(time.Second),
	)

	// 获取配置摘要
	configSummary, err := o.mcpClient.GetSummary(ctx)
	if err != nil {
		slog.Warn("[Orch] 获取 MCP 摘要失败", "err", err)
		configSummary = validationSummary
	}
	o.state.ConfigSummary = configSummary

	// ══════════════════════════════════════════════════════
	// Step 4: 展示结果
	// ══════════════════════════════════════════════════════
	logger.Phase("阶段 4/4", "展示结果")
	o.state.Phase = PhaseIDFReady

	// 展示 MCP 配置摘要
	if configSummary != "" {
		ui.PrintSummary(configSummary)
	}

	// 展示 YAML 文件内容（前 80 行）
	ui.PrintYAMLContent(yamlPath, 80)

	// 展示最终完成横幅
	o.state.Phase = PhaseDone
	totalDuration := o.state.TotalDuration().Round(time.Second).String()
	ui.PrintFinalResult(yamlPath, totalDuration)

	slog.Info("[Orch] 会话完成",
		"session_id", o.state.SessionID,
		"total_duration", totalDuration,
		"yaml_path", yamlPath,
	)

	return nil
}

// runMCPValidation 执行 MCP 加载 + 验证，带错误处理
func (o *Orchestrator) runMCPValidation(ctx context.Context, yamlPath string) (string, error) {
	// 3.1 加载 YAML
	slog.Info("[Orch] MCP: 加载 YAML", "path", yamlPath)
	if err := o.mcpClient.LoadYAML(ctx, yamlPath); err != nil {
		return "", fmt.Errorf("加载失败: %w", err)
	}
	ui.PrintInfo("YAML 已加载到 MCP Server")

	// 3.2 验证配置
	slog.Info("[Orch] MCP: 验证配置")
	validateResult, err := o.mcpClient.ValidateConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("验证失败: %w", err)
	}

	return validateResult, nil
}

// State 返回当前会话状态（只读）
func (o *Orchestrator) State() *SessionState {
	return o.state
}
