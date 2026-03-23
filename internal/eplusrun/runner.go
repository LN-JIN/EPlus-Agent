// EPlus-MCP CLI 直接调用封装
// 通过子进程执行 EPlus-MCP 的 main.py 各命令，代替通过 MCP HTTP 协议调用。
// 这样可绕过 MCP 调用仿真时出现的未知异常，同时保持相同的功能接口。
//
// 依赖: EPlus-MCP main.py 存在于 scriptPath，且 Python 环境已激活。
// 命令工作目录设为 main.py 所在目录（确保相对路径正确解析）。

package eplusrun

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner 封装对 EPlus-MCP CLI (main.py) 的调用
type Runner struct {
	scriptPath string // main.py 的绝对路径
	workDir    string // 命令执行目录（main.py 所在目录）
	pythonCmd  string // 解析后的 Python 可执行路径
}

// NewRunner 创建 Runner
// scriptPath: EPlus-MCP main.py 的绝对路径（如 D:\TryAgent\EPlus-MCP\main.py）
// pythonPath: Python 解释器路径，留空则自动探测 python → python3
func NewRunner(scriptPath, pythonPath string) *Runner {
	return &Runner{
		scriptPath: scriptPath,
		workDir:    filepath.Dir(scriptPath),
		pythonCmd:  resolvePython(pythonPath),
	}
}

// resolvePython 解析 Python 可执行路径
// 若 configured 非空则直接使用；否则按顺序尝试 python、python3
func resolvePython(configured string) string {
	if configured != "" {
		return configured
	}
	for _, candidate := range []string{"python", "python3"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return "python" // 兜底，调用时会产生明确错误
}

// Probe 检查 Python 环境和脚本文件是否就绪，在启动时调用一次
func (r *Runner) Probe() error {
	// 检查 Python 可执行
	cmd := exec.Command(r.pythonCmd, "--version")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf(
			"Python 环境不可用 (命令: %q, 错误: %v)\n输出: %s\n"+
				"请确认 Python 已安装并在 PATH 中，或在配置文件中指定 session.python_path",
			r.pythonCmd, err, strings.TrimSpace(string(out)),
		)
	}
	// 检查 EPlus-MCP 脚本文件
	if _, err := os.Stat(r.scriptPath); err != nil {
		return fmt.Errorf("EPlus-MCP 脚本不存在: %s\n请确认 session.simulation_script 路径正确", r.scriptPath)
	}
	return nil
}

// runCommand 执行 python main.py <args> 并返回完整 stdout
func (r *Runner) runCommand(ctx context.Context, args ...string) (string, error) {
	cmdArgs := append([]string{r.scriptPath}, args...)
	cmd := exec.CommandContext(ctx, r.pythonCmd, cmdArgs...)
	cmd.Dir = r.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Debug("[eplusrun] 执行命令", "args", strings.Join(args, " "))

	err := cmd.Run()
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	if stderrStr != "" {
		slog.Debug("[eplusrun] stderr", "content", stderrStr)
	}

	if err != nil {
		errDetail := extractError(stdoutStr)
		if errDetail == "" {
			errDetail = stderrStr
		}
		return stdoutStr, fmt.Errorf("命令失败 (exit %v): %s", err, errDetail)
	}

	return stdoutStr, nil
}

// ConvertYAMLToIDF 将 YAML 配置文件转换为 EnergyPlus IDF 文件
// yamlPath: 输入 YAML 文件路径
// outputDir: IDF 输出目录（空字符串使用 EPlus-MCP 默认值 ./output/idf/）
// 返回: 生成的 IDF 文件绝对路径
func (r *Runner) ConvertYAMLToIDF(ctx context.Context, yamlPath, outputDir string) (string, error) {
	slog.Info("[eplusrun] YAML→IDF 转换", "yaml", yamlPath, "output_dir", outputDir)

	args := []string{"convert-idf", yamlPath}
	if outputDir != "" {
		args = append(args, "--output-dir", outputDir)
	}

	stdout, err := r.runCommand(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("YAML→IDF 转换失败: %w", err)
	}

	idfPath, ok := parseMarker(stdout, markerIDFOutput)
	if !ok {
		return "", fmt.Errorf("YAML→IDF 转换输出中未找到 IDF_OUTPUT 标记\nstdout: %s", stdout)
	}

	slog.Info("[eplusrun] IDF 生成成功", "idf_path", idfPath)
	return idfPath, nil
}

// RunSimulation 执行 EnergyPlus 仿真
// idfPath: IDF 文件路径
// epwPath: EPW 气象文件路径
// outputDir: 仿真输出目录（空字符串使用默认值 ./output/results/{stem}/）
// 返回: 仿真输出目录绝对路径
func (r *Runner) RunSimulation(ctx context.Context, idfPath, epwPath, outputDir string) (string, error) {
	slog.Info("[eplusrun] 仿真运行", "idf", idfPath, "epw", epwPath)

	args := []string{"run-simulation", idfPath}
	if epwPath != "" {
		args = append(args, "--epw-file", epwPath)
	}
	if outputDir != "" {
		args = append(args, "--output-dir", outputDir)
	}

	stdout, err := r.runCommand(ctx, args...)
	if err != nil {
		// 即使仿真失败，仍尝试提取输出目录（用于读取错误日志）
		simDir, _ := parseMarker(stdout, markerSimOutDir)
		return simDir, fmt.Errorf("仿真运行失败: %w", err)
	}

	simOutDir, ok := parseMarker(stdout, markerSimOutDir)
	if !ok {
		return "", fmt.Errorf("仿真输出中未找到 SIMULATION_OUTPUT_DIR 标记\nstdout: %s", stdout)
	}

	slog.Info("[eplusrun] 仿真完成", "sim_out_dir", simOutDir)
	return simOutDir, nil
}

// ValidateIDF 验证 IDF 文件语法合法性（不执行仿真）
// 返回: (是否合法, 详细信息)
func (r *Runner) ValidateIDF(ctx context.Context, idfPath string) (bool, string, error) {
	slog.Info("[eplusrun] 验证 IDF", "idf", idfPath)

	stdout, err := r.runCommand(ctx, "validate-idf", idfPath)
	if err != nil {
		// validate-idf 失败本身不是 Go 错误，通过 IDF_VALID 标记报告
		if v, ok := parseMarker(stdout, markerIDFValid); ok {
			return false, v, nil
		}
		return false, err.Error(), nil
	}

	v, ok := parseMarker(stdout, markerIDFValid)
	if !ok {
		return false, stdout, nil
	}

	valid := strings.HasPrefix(v, "True")
	return valid, v, nil
}

// ReadIDFObjects 读取 IDF 文件中指定类型的所有对象名称和字段值
// 返回 JSON 字符串（[]{"name":..., "fields":{...}}），失败返回 "[]"
func (r *Runner) ReadIDFObjects(ctx context.Context, idfPath, objectType string) (string, error) {
	slog.Info("[eplusrun] 读取 IDF 对象", "idf", idfPath, "object_type", objectType)

	stdout, err := r.runCommand(ctx, "read-idf", idfPath, "--object-type", objectType)
	if err != nil {
		// 命令失败时返回空数组而非错误，让 Planner 继续工作
		slog.Warn("[eplusrun] read-idf 命令失败，返回空列表", "err", err)
		return "[]", nil
	}

	jsonStr, ok := parseMarker(stdout, markerReadIDF)
	if !ok {
		return "[]", nil
	}

	slog.Info("[eplusrun] IDF 对象读取完成", "object_type", objectType, "result_len", len(jsonStr))
	return jsonStr, nil
}

// EditIDF 修改 IDF 文件中指定对象的字段值
// objectType: EnergyPlus 对象类型（如 "ZoneHVAC:IdealLoadsAirSystem"）
// name: 对象 Name 字段的值
// field: 要修改的 eppy 属性名
// value: 新值（字符串，eppy 自动转型）
func (r *Runner) EditIDF(ctx context.Context, idfPath, objectType, name, field, value string) error {
	slog.Info("[eplusrun] 修改 IDF 对象",
		"idf", idfPath,
		"object_type", objectType,
		"name", name,
		"field", field,
		"value", value,
	)

	stdout, err := r.runCommand(ctx,
		"edit-idf", idfPath,
		"--object-type", objectType,
		"--name", name,
		"--field", field,
		"--value", value,
	)
	if err != nil {
		return fmt.Errorf("IDF 修改失败: %w", err)
	}

	if _, ok := parseMarker(stdout, markerEditOK); !ok {
		return fmt.Errorf("IDF 修改输出异常: %s", stdout)
	}

	slog.Info("[eplusrun] IDF 修改成功")
	return nil
}
