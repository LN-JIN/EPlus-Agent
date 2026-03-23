// 工具注册表模块
// 统一管理 LLM 可调用的所有工具。每个工具包含 OpenAI function calling 格式的定义
// 和对应的 Go 执行函数（Handler）。模块通过 Register 注册，通过 Execute 执行。
// 各业务模块（意图收集、YAML 生成等）分别注册自己的工具集到同一 Registry 实例。

package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"energyplus-agent/internal/llm"
)

// Handler 工具执行函数类型
// args: 已解析的 JSON 参数（map 形式）
// 返回: 工具输出字符串（作为 Observation 传回 LLM）
type Handler func(args map[string]any) (string, error)

// ToolEntry 工具的完整描述（定义 + 执行函数）
type ToolEntry struct {
	Tool    llm.Tool
	Handler Handler
}

// Registry 工具注册表
type Registry struct {
	entries map[string]*ToolEntry
}

// NewRegistry 创建空的工具注册表
func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]*ToolEntry),
	}
}

// Register 注册一个工具
// tool: LLM 可见的工具定义（含参数 Schema）
// handler: 实际执行函数
func (r *Registry) Register(tool llm.Tool, handler Handler) {
	r.entries[tool.Function.Name] = &ToolEntry{
		Tool:    tool,
		Handler: handler,
	}
}

// Execute 执行工具调用
// name: 工具名称
// argsJSON: LLM 返回的 JSON 格式参数字符串
func (r *Registry) Execute(name, argsJSON string) (string, error) {
	entry, ok := r.entries[name]
	if !ok {
		return "", fmt.Errorf("未知工具: %s（已注册工具: %v）", name, r.Names())
	}

	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("解析工具参数失败 [%s]: %w\n原始参数: %s", name, err, argsJSON)
		}
	}
	if args == nil {
		args = make(map[string]any)
	}

	result, err := entry.Handler(args)
	if err != nil {
		// 将错误作为 Observation 返回给 LLM，让其自行决策处理
		return fmt.Sprintf("ERROR: %v", err), nil
	}
	return result, nil
}

// ToLLMTools 将注册的工具转换为 LLM 可用格式
func (r *Registry) ToLLMTools() []llm.Tool {
	tools := make([]llm.Tool, 0, len(r.entries))
	for _, entry := range r.entries {
		tools = append(tools, entry.Tool)
	}
	return tools
}

// Names 返回所有已注册的工具名称
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	return names
}

// ============================================================
// 工具定义辅助函数（减少重复的 JSON Schema 样板代码）
// ============================================================

// StringParam 构建字符串类型参数的 JSON Schema
func StringParam(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

// NumberParam 构建数字类型参数的 JSON Schema
func NumberParam(description string) map[string]any {
	return map[string]any{
		"type":        "number",
		"description": description,
	}
}

// ObjectSchema 构建对象类型参数的 JSON Schema
func ObjectSchema(description string, properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":        "object",
		"description": description,
		"properties":  properties,
		"required":    required,
	}
}

// GetString 从 args 中安全获取字符串值
func GetString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("缺少必填参数: %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("参数 %s 类型错误，期望 string，实际 %T", key, v)
	}
	return s, nil
}

// GenerateToolDescriptions 从注册的工具列表动态生成 Markdown 格式的工具描述。
// 格式: "- tool_name(param1, param2): description"
// 用于替换 System Prompt 中的 {tool_descriptions} 占位符，避免提示词硬编码工具列表。
func (r *Registry) GenerateToolDescriptions() string {
	if len(r.entries) == 0 {
		return "(no tools registered)"
	}

	var sb strings.Builder
	// 按名称排序，保证输出稳定
	names := r.Names()
	sortStrings(names)
	for _, name := range names {
		entry := r.entries[name]
		fn := entry.Tool.Function

		// 提取必填参数列表
		params := extractRequiredParams(fn.Parameters)

		sb.WriteString("- ")
		sb.WriteString(name)
		sb.WriteString("(")
		sb.WriteString(strings.Join(params, ", "))
		sb.WriteString("): ")
		sb.WriteString(fn.Description)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// sortStrings 对字符串切片原地排序（避免引入 sort 包以外的依赖）
func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

// extractRequiredParams 从 JSON Schema 中提取 required 参数名列表
func extractRequiredParams(schema any) []string {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	req, ok := m["required"]
	if !ok {
		return nil
	}
	reqSlice, ok := req.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(reqSlice))
	for _, v := range reqSlice {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// GetStringOr 从 args 中获取字符串值，不存在时返回默认值
func GetStringOr(args map[string]any, key, defaultVal string) string {
	v, ok := args[key]
	if !ok {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}
