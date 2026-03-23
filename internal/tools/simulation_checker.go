// 仿真结果检查工具
// 检查 EnergyPlus 仿真输出目录的关键结果文件，判断仿真是否成功。
// 同时提供 LLM Tool 定义和 Handler，供仿真相关模块注册到工具注册表。

package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"energyplus-agent/internal/llm"
)

// SimCheckResult 仿真结果检查输出
type SimCheckResult struct {
	OutputDir    string   `json:"output_dir"`
	DirExists    bool     `json:"dir_exists"`    // 输出目录本身是否存在
	Success      bool     `json:"success"`
	EndMessage   string   `json:"end_message"`   // eplusout.end 的内容摘要
	ErrSummary   string   `json:"err_summary"`   // eplusout.err 中 Fatal/Severe 摘要（最多 50 行）
	FilesFound   []string `json:"files_found"`   // 已生成的关键文件
	FilesMissing []string `json:"files_missing"` // 缺失的关键文件
}

// keyFiles 仿真完成后应存在的关键输出文件
var keyFiles = []string{
	"eplusout.end",
	"eplusout.err",
	"eplusout.eso",
	"eplusout.csv",
}

// CheckSimulationResult 检查仿真输出目录，返回成功/失败状态及摘要
func CheckSimulationResult(outputDir string) (*SimCheckResult, error) {
	res := &SimCheckResult{
		OutputDir:    outputDir,
		FilesFound:   make([]string, 0),
		FilesMissing: make([]string, 0),
	}

	// 检查目录本身是否存在
	if info, err := os.Stat(outputDir); err == nil && info.IsDir() {
		res.DirExists = true
	}

	// 检查关键文件存在性
	for _, f := range keyFiles {
		path := filepath.Join(outputDir, f)
		if _, err := os.Stat(path); err == nil {
			res.FilesFound = append(res.FilesFound, f)
		} else {
			res.FilesMissing = append(res.FilesMissing, f)
		}
	}

	// 读取 eplusout.end 判断成功/失败
	endPath := filepath.Join(outputDir, "eplusout.end")
	if endData, err := os.ReadFile(endPath); err == nil {
		endMsg := strings.TrimSpace(string(endData))
		res.EndMessage = endMsg
		res.Success = strings.Contains(endMsg, "EnergyPlus Completed Successfully")
	}

	// 读取 eplusout.err 提取 Fatal/Severe 摘要
	errPath := filepath.Join(outputDir, "eplusout.err")
	if errData, err := os.ReadFile(errPath); err == nil {
		res.ErrSummary = extractErrSummary(string(errData))
	}

	return res, nil
}

// extractErrSummary 从 eplusout.err 内容中提取 Fatal/Severe 错误行（最多 50 行）
func extractErrSummary(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		upper := strings.ToUpper(line)
		if strings.Contains(upper, "FATAL") || strings.Contains(upper, "SEVERE") {
			lines = append(lines, strings.TrimSpace(line))
			if len(lines) >= 50 {
				lines = append(lines, "...(truncated)")
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

// ── LLM Tool 定义与 Handler ─────────────────────────────────────────────────

// SimCheckToolDef 返回 check_simulation_result 的 LLM 工具定义
func SimCheckToolDef() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "check_simulation_result",
			Description: "检查 EnergyPlus 仿真输出目录中的关键结果文件，判断仿真是否成功，并提取错误摘要（Fatal/Severe）",
			Parameters: ObjectSchema(
				"仿真结果检查参数",
				map[string]any{
					"output_dir": StringParam("仿真输出目录路径（包含 eplusout.end/err/eso/csv 等文件）"),
				},
				[]string{"output_dir"},
			),
		},
	}
}

// SimCheckHandler 返回 check_simulation_result 的执行 Handler
func SimCheckHandler() Handler {
	return func(args map[string]any) (string, error) {
		outputDir, err := GetString(args, "output_dir")
		if err != nil {
			return "", err
		}

		result, err := CheckSimulationResult(outputDir)
		if err != nil {
			return fmt.Sprintf("检查失败: %v", err), nil
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Sprintf("序列化失败: %v", err), nil
		}
		return string(data), nil
	}
}
