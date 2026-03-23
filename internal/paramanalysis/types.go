// Phase 6 参数分析核心数据类型
// IDFEdit 描述对 IDF 文件的单次字段修改；
// ParamVariation 是一组修改的组合（参数变体方案）；
// WorkerResult 是单个 Worker 的执行结果。

package paramanalysis

// IDFEdit 描述对 IDF 文件中单个对象字段的修改
// ObjectType/Name 定位目标对象（与 eppy 的 idfobjects[ObjectType]/Name 对应）
// Field 是 eppy 属性名（区分大小写），Value 是新值字符串（eppy 自动类型转换）
type IDFEdit struct {
	ObjectType string `json:"object_type"` // e.g. "Material"
	Name       string `json:"name"`        // 对象的 Name 字段值（从 list_idf_objects 获取）
	Field      string `json:"field"`       // eppy 属性名，e.g. "Thickness"
	Value      string `json:"value"`       // 新值字符串，e.g. "0.05"
}

// ParamVariation 一个参数变体方案（零个或多个 IDFEdit 的组合）
// Label 用作目录名和报告中的标识符（slug 格式，如 "wall_ins_5cm"）
// baseline 变体的 Edits 为空（零编辑，与基础 IDF 完全相同）
type ParamVariation struct {
	Label       string    `json:"label"`       // 变体标识符，e.g. "baseline"、"wall_ins_5cm"
	Description string    `json:"description"` // 自然语言描述，e.g. "外墙保温厚度 5cm"
	Edits       []IDFEdit `json:"edits"`       // 对基础 IDF 的修改列表（空 = baseline）
}

// WorkerResult 单个 Worker 的执行结果
type WorkerResult struct {
	Label       string             `json:"label"`
	Description string             `json:"description"`
	Success     bool               `json:"success"`
	FixAttempts int                `json:"fix_attempts"` // 仿真修复尝试次数
	TokensUsed  int                `json:"tokens_used"`  // 本次 Worker 消耗的 token 数
	Metrics     map[string]float64 `json:"metrics"`      // 提取的能耗指标（列名→值）
	SimOutDir   string             `json:"sim_out_dir"`  // 仿真输出目录
	Error       string             `json:"error,omitempty"`
}
