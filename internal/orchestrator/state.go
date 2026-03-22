// Orchestrator 状态定义模块
// 定义 Agent 会话的完整状态机，包括各阶段枚举、快照记录和会话上下文。
// SessionState 贯穿整个 "意图收集 → YAML 生成 → MCP 转换 → 展示 IDF" 流程，
// 每个阶段完成后更新状态，方便断点续传和问题定位。

package orchestrator

import (
	"time"

	"energyplus-agent/internal/intent"
)

// Phase 流程阶段枚举
type Phase string

const (
	// PhaseIntentCollection 阶段 1：与用户多轮对话，收集建筑设计意图
	PhaseIntentCollection Phase = "intent_collection"
	// PhaseYAMLGenerating 阶段 2：LLM 根据意图生成 EnergyPlus YAML 配置
	PhaseYAMLGenerating Phase = "yaml_generating"
	// PhaseIDFConverting 阶段 3：调用 MCP Server 加载并验证 YAML
	PhaseIDFConverting Phase = "idf_converting"
	// PhaseIDFReady 阶段 4：配置验证通过，展示结果
	PhaseIDFReady Phase = "idf_ready"
	// PhaseDone 全流程完成
	PhaseDone Phase = "done"
	// PhaseCancelled 用户取消
	PhaseCancelled Phase = "cancelled"
	// PhaseFailed 流程异常终止
	PhaseFailed Phase = "failed"
)

// String 返回阶段的中文描述
func (p Phase) String() string {
	switch p {
	case PhaseIntentCollection:
		return "意图收集"
	case PhaseYAMLGenerating:
		return "YAML 生成"
	case PhaseIDFConverting:
		return "MCP 配置验证"
	case PhaseIDFReady:
		return "IDF 就绪"
	case PhaseDone:
		return "完成"
	case PhaseCancelled:
		return "已取消"
	case PhaseFailed:
		return "失败"
	default:
		return string(p)
	}
}

// IDFSnapshot 某个时间点的配置快照记录
type IDFSnapshot struct {
	Label     string    // 快照标签（如 "initial", "after_fix_1"）
	Phase     Phase     // 生成时所处阶段
	YAMLPath  string    // 对应的 YAML 文件路径
	CreatedAt time.Time // 快照时间
}

// SessionState 完整会话状态
type SessionState struct {
	// 唯一会话标识
	SessionID string

	// 当前所处阶段
	Phase Phase

	// 意图收集结果（Phase >= PhaseYAMLGenerating 后有效）
	Intent *intent.BuildingIntent

	// 生成的 YAML 文件路径（Phase >= PhaseIDFConverting 后有效）
	YAMLPath string

	// MCP 验证摘要（Phase >= PhaseIDFReady 后有效）
	ValidationSummary string

	// MCP 配置摘要（Phase == PhaseIDFReady 后有效）
	ConfigSummary string

	// 历史快照列表（用于调试和回溯）
	Snapshots []IDFSnapshot

	// 错误信息（Phase == PhaseFailed 时有效）
	FailureReason string

	// 会话开始时间
	CreatedAt time.Time

	// 各阶段完成时间记录
	IntentDoneAt    time.Time
	YAMLDoneAt      time.Time
	ConvertDoneAt   time.Time
}

// NewSessionState 创建新的会话状态
func NewSessionState(sessionID string) *SessionState {
	return &SessionState{
		SessionID: sessionID,
		Phase:     PhaseIntentCollection,
		Snapshots: make([]IDFSnapshot, 0),
		CreatedAt: time.Now(),
	}
}

// AddSnapshot 添加配置快照
func (s *SessionState) AddSnapshot(label, yamlPath string) {
	s.Snapshots = append(s.Snapshots, IDFSnapshot{
		Label:     label,
		Phase:     s.Phase,
		YAMLPath:  yamlPath,
		CreatedAt: time.Now(),
	})
}

// TotalDuration 返回从会话开始到当前的总耗时
func (s *SessionState) TotalDuration() time.Duration {
	return time.Since(s.CreatedAt)
}
