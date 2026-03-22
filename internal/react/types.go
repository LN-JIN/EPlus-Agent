// ReAct 模式数据类型定义
// ReAct (Reasoning + Acting) 是本项目所有 LLM 交互的核心模式。
// 每次 LLM 调用的完整过程被记录为一组 Step，方便追踪和调试。

package react

// Step 单次 ReAct 循环的执行步骤记录
type Step struct {
	Iter        int    // 当前迭代轮次（从 1 开始）
	Thought     string // LLM 的思考内容（文本响应部分）
	Action      string // 调用的工具名称（空表示直接回答）
	ActionInput string // 工具调用的参数（JSON 字符串）
	Observation string // 工具执行的返回结果
	IsFinal     bool   // 是否为最终回答步骤
	FinalAnswer string // 最终回答内容（IsFinal=true 时有效）
}

// Result ReAct 运行的最终结果
type Result struct {
	Steps       []Step // 完整的推理步骤记录
	FinalAnswer string // 最终回答
	Error       error  // 运行时错误（nil 表示成功）
}

// Summary 返回 Result 的简要摘要（用于日志）
func (r *Result) Summary() string {
	if r.Error != nil {
		return "ReAct 失败: " + r.Error.Error()
	}
	return "ReAct 完成，共 " + itoa(len(r.Steps)) + " 步，最终回答长度=" + itoa(len(r.FinalAnswer))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
