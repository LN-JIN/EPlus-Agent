// 跨模块共享的会话状态定义
// SessionState 贯穿所有阶段（意图收集 → 参数分析），各模块通过此包访问状态，
// 避免业务模块与 orchestrator 包之间的循环导入。

package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Phase 流程阶段枚举
type Phase string

const (
	// PhaseIntentCollection 阶段 1：与用户多轮对话，收集建筑设计意图
	PhaseIntentCollection Phase = "intent_collection"
	// PhaseYAMLGenerating 阶段 2：LLM 根据意图生成 EnergyPlus YAML 配置
	PhaseYAMLGenerating Phase = "yaml_generating"
	// PhaseIDFConverting 阶段 3：YAML→IDF 自愈转换
	PhaseIDFConverting Phase = "idf_converting"
	// PhaseSimRunning 阶段 4：EnergyPlus 仿真（带 ReAct 修复）
	PhaseSimRunning Phase = "sim_running"
	// PhaseReportReading 阶段 5：读取仿真结果，生成报告
	PhaseReportReading Phase = "report_reading"
	// PhaseParamAnalysis 阶段 6：Planner + Worker 参数分析
	PhaseParamAnalysis Phase = "param_analysis"
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
		return "YAML→IDF 转换"
	case PhaseSimRunning:
		return "仿真运行"
	case PhaseReportReading:
		return "报告解读"
	case PhaseParamAnalysis:
		return "参数分析"
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
	Label     string    `json:"label"`      // 快照标签（如 "initial_yaml", "v2_after_fix"）
	Phase     Phase     `json:"phase"`      // 生成时所处阶段
	YAMLPath  string    `json:"yaml_path"`  // 对应的 YAML 文件路径（可为空）
	IDFPath   string    `json:"idf_path"`   // 对应的 IDF 文件路径（可为空）
	SimOutDir string    `json:"sim_out_dir"` // 对应的仿真输出目录（可为空）
	CreatedAt time.Time `json:"created_at"` // 快照时间
}

// SessionState 完整会话状态（贯穿所有阶段）
type SessionState struct {
	mu sync.Mutex `json:"-"` // 保护 token 字段的并发写入

	// 唯一会话标识
	SessionID string `json:"session_id"`

	// 当前所处阶段
	Phase Phase `json:"phase"`

	// ── 阶段产物（随阶段递进填充）──────────────────────────────
	// YAMLPath 生成的 YAML 文件路径（Phase 2 后有效）
	YAMLPath string `json:"yaml_path"`
	// IDFPath 当前有效 IDF 文件路径（Phase 3 后有效）
	IDFPath string `json:"idf_path"`
	// SimOutDir 当前仿真输出目录（Phase 4 后有效）
	SimOutDir string `json:"sim_out_dir"`
	// ReportPath Phase 5 生成的 Markdown 报告路径
	ReportPath string `json:"report_path"`
	// ParamReportPath Phase 6 生成的参数分析报告路径
	ParamReportPath string `json:"param_report_path"`

	// ── 用户初始输入（在 RunWithConfig 时设置）──────────────────────────
	UserInput string `json:"user_input"`

	// ── 意图数据（Phase 1 后有效）────────────────────────────────────
	// IntentJSON BuildingIntent 的 JSON 序列化（Phase 2 反序列化使用）
	IntentJSON string `json:"intent_json"`
	// IntentSummary 意图的自然语言摘要（Phase 1 后有效，后续所有阶段注入 SystemPrompt）
	IntentSummary string `json:"intent_summary"`

	// ── 用户请求的输出变量（Phase 1 收集，Phase 3 写入 IDF）──────────
	// 预定义值: "hvac_energy"、"zone_temperature"；或 LLM 解析的 EnergyPlus variable name
	OutputVariables []string `json:"output_variables"`

	// ── MCP 验证摘要（向后兼容 Phase 3 旧逻辑）───────────────────────
	ValidationSummary string `json:"validation_summary"`
	ConfigSummary     string `json:"config_summary"`

	// ── 版本快照（全局记录所有迭代历史）─────────────────────────────
	Snapshots []IDFSnapshot `json:"snapshots"`

	// ── Token 消耗追踪 ─────────────────────────────────────────────
	TotalTokens int            `json:"total_tokens"`
	PhaseTokens map[Phase]int  `json:"phase_tokens"`

	// ── 错误信息（Phase == PhaseFailed 时有效）────────────────────────
	FailureReason string `json:"failure_reason"`

	// ── 时间戳 ────────────────────────────────────────────────────
	CreatedAt      time.Time `json:"created_at"`
	IntentDoneAt   time.Time `json:"intent_done_at"`
	YAMLDoneAt     time.Time `json:"yaml_done_at"`
	ConvertDoneAt  time.Time `json:"convert_done_at"`
	SimDoneAt      time.Time `json:"sim_done_at"`
	ReportDoneAt   time.Time `json:"report_done_at"`
	ParamDoneAt    time.Time `json:"param_done_at"`
}

// NewSessionState 创建新的会话状态
func NewSessionState(sessionID string) *SessionState {
	return &SessionState{
		SessionID:   sessionID,
		Phase:       PhaseIntentCollection,
		Snapshots:   make([]IDFSnapshot, 0),
		PhaseTokens: make(map[Phase]int),
		CreatedAt:   time.Now(),
	}
}

// AddSnapshot 添加配置快照（YAML 快照）
func (s *SessionState) AddSnapshot(label, yamlPath string) {
	s.Snapshots = append(s.Snapshots, IDFSnapshot{
		Label:     label,
		Phase:     s.Phase,
		YAMLPath:  yamlPath,
		CreatedAt: time.Now(),
	})
}

// AddIDFSnapshot 添加 IDF 快照
func (s *SessionState) AddIDFSnapshot(label, idfPath, simOutDir string) {
	s.Snapshots = append(s.Snapshots, IDFSnapshot{
		Label:     label,
		Phase:     s.Phase,
		IDFPath:   idfPath,
		SimOutDir: simOutDir,
		CreatedAt: time.Now(),
	})
}

// AddTokens 累加 token 消耗（线程安全）
func (s *SessionState) AddTokens(count int) {
	if count == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalTokens += count
	if s.PhaseTokens == nil {
		s.PhaseTokens = make(map[Phase]int)
	}
	s.PhaseTokens[s.Phase] += count
}

// TotalDuration 返回从会话开始到当前的总耗时
func (s *SessionState) TotalDuration() time.Duration {
	return time.Since(s.CreatedAt)
}

// SaveToFile 将会话状态持久化到 JSON 文件（断点续传支持）
func (s *SessionState) SaveToFile(outputDir string) error {
	dir := filepath.Join(outputDir, "session")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 session 目录失败: %w", err)
	}
	path := filepath.Join(dir, s.SessionID+".json")
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 session 状态失败: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadFromFile 从 JSON 文件恢复会话状态
func LoadFromFile(outputDir, sessionID string) (*SessionState, error) {
	path := filepath.Join(outputDir, "session", sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 session 文件失败 [%s]: %w", path, err)
	}
	s := &SessionState{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("解析 session 文件失败: %w", err)
	}
	return s, nil
}
