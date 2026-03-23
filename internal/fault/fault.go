// Package fault 提供统一的错误分类，供各重试循环判断是否继续重试。
// 避免在每个模块中重复实现 isFatalError() 等判断逻辑。
//
// 分类层次：
//   KindEnvironment — Python/脚本/文件缺失，致命，立即终止
//   KindContent     — YAML/IDF 内容错误，LLM 修复后可重试
//   KindSimulation  — EnergyPlus Fatal/Severe，ReAct 修复后可重试
//   KindTransient   — API 超时/限流，稍后重试
//   KindUnknown     — 无法分类，按上限重试

package fault

import "strings"

// ErrorKind 错误分类
type ErrorKind int

const (
	KindUnknown     ErrorKind = iota // 无法分类，默认可重试
	KindEnvironment                  // 环境配置问题 — 致命，立即终止
	KindContent                      // YAML/IDF 内容问题 — 可通过 LLM 修复
	KindSimulation                   // EnergyPlus 仿真错误 — 可通过 ReAct 修复
	KindTransient                    // API 超时/限流 — 稍后重试
)

// Classify 根据 error 分类错误类型
func Classify(err error) ErrorKind {
	if err == nil {
		return KindUnknown
	}
	return ClassifyMsg(err.Error())
}

// ClassifyMsg 根据错误消息字符串分类错误类型
func ClassifyMsg(msg string) ErrorKind {
	// 环境致命错误 — 修改配置才能解决，LLM 修复 YAML/IDF 无效
	if containsAny(msg,
		// Windows: Python 未安装（exit 9009）
		"Python was not found",
		"exit status 9009",
		// Linux/Mac: python 不在 PATH
		"python: command not found",
		"python3: command not found",
		// eplusrun.Runner.Probe() 返回的消息
		"Python 环境不可用",
		"EPlus-MCP 脚本不存在",
		// idfconvert 封装的环境错误
		"环境配置错误",
	) {
		return KindEnvironment
	}

	// 网络/API 临时错误
	if containsAny(msg, "timeout", "deadline exceeded", "rate limit", "429", "503") {
		return KindTransient
	}

	// EnergyPlus 仿真错误（来自 eplusout.err）
	if containsAny(msg, "仿真运行失败", "仿真失败") {
		return KindSimulation
	}

	// YAML/IDF 内容错误（来自 EPlus-MCP CLI）
	if containsAny(msg, "YAML→IDF 转换失败", "命令失败") {
		return KindContent
	}

	return KindUnknown
}

// IsFatal 返回 true 表示错误为致命错误，重试无法解决，应立即终止。
func IsFatal(err error) bool {
	return Classify(err) == KindEnvironment
}

// IsRetryable 返回 true 表示错误可能通过重试（或 LLM 修复后重试）解决。
func IsRetryable(err error) bool {
	return Classify(err) != KindEnvironment
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
