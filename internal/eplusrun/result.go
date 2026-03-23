// EPlus-MCP CLI 输出解析
// EPlus-MCP 的各命令通过 stdout 打印机器可读的标记行，此文件负责解析这些标记。
// 标记格式: "MARKER_NAME: value"，其余行为日志信息（忽略）。

package eplusrun

import (
	"strings"
)

const (
	markerIDFOutput    = "IDF_OUTPUT:"
	markerSimOutDir    = "SIMULATION_OUTPUT_DIR:"
	markerIDFValid     = "IDF_VALID:"
	markerEditOK       = "EDIT_OK:"
	markerReadIDF      = "READ_IDF_OBJECTS:"
	markerIDFError     = "IDF_ERROR:"
	markerSimError     = "SIMULATION_ERROR:"
	markerEditError    = "EDIT_ERROR:"
)

// parseMarker 从 stdout 中提取指定标记后的值。
// 遍历所有行，返回第一个匹配行的值部分（去除前缀和首尾空格）。
func parseMarker(stdout, marker string) (string, bool) {
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, marker) {
			return strings.TrimSpace(strings.TrimPrefix(line, marker)), true
		}
	}
	return "", false
}

// extractError 从 stdout 中提取错误标记内容（用于错误场景的详细信息）。
func extractError(stdout string) string {
	for _, errMarker := range []string{markerIDFError, markerSimError, markerEditError} {
		if v, ok := parseMarker(stdout, errMarker); ok {
			return v
		}
	}
	return ""
}
