// RunSimulation Demo
// 演示如何直接调用 EnergyPlus 对已展开的 IDF 文件进行仿真，无需经过 MCP Server。
//
// 适用场景：IDF 已通过 ExpandObjects 展开（不含 HVACTemplate），可直接传给 EnergyPlus。
//
// 用法：
//
//	go run cmd/run_simulation_demo/main.go [expanded.idf] [weather.epw]
//
// 若不传参数，使用默认路径。
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ── 路径默认值 ────────────────────────────────────────────────

const (
	defaultIDF     = `D:\TryAgent\energyplus-agent\output\simulation\expand_output\temp_20260320_143304_expanded.idf`
	defaultEPW     = `D:\TryAgent\EnergyPlus-Agent-try2\data\weather\Shenzhen.epw`
	defaultOutBase = `D:\TryAgent\energyplus-agent\output\simulation\run_demo`
)

// ── EnergyPlus 自动检测 ───────────────────────────────────────

func findEnergyPlus() (string, error) {
	if exe := os.Getenv("ENERGYPLUS_EXE"); exe != "" {
		return exe, nil
	}
	candidates := []string{
		`D:\programs\EnergyPlusV25-2-0\energyplus.exe`,
		`C:\EnergyPlusV25-2-0\energyplus.exe`,
		`C:\EnergyPlusV24-2-0\energyplus.exe`,
		`/usr/local/EnergyPlus-25-2-0/energyplus`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	name := "energyplus"
	if runtime.GOOS == "windows" {
		name = "energyplus.exe"
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("找不到 EnergyPlus 可执行文件，请设置环境变量 ENERGYPLUS_EXE")
}

// ── SimResult 仿真结果摘要 ────────────────────────────────────

type SimResult struct {
	OutputDir   string
	ErrFile     string
	Fatals      []string
	Severes     []string
	Warnings    []string
	Elapsed     time.Duration
	Success     bool
}

// ── RunSimulation 直接调用 EnergyPlus ────────────────────────

// RunSimulation 对已展开的 IDF 文件运行 EnergyPlus 仿真。
//
// idfPath:   输入的 IDF 文件（已通过 ExpandObjects 展开）
// epwPath:   EPW 气象文件路径（为空则仅运行 design-day）
// outputDir: 仿真输出目录
func RunSimulation(idfPath, epwPath, outputDir string) (*SimResult, error) {
	exePath, err := findEnergyPlus()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 构建命令
	// -d: 输出目录  -w: 气象文件  -r: 运行 ReadVarsESO 生成 CSV
	args := []string{
		"-d", outputDir,
		"-r", // 生成 CSV
	}
	if epwPath != "" {
		args = append(args, "-w", epwPath)
	} else {
		args = append(args, "--design-day")
	}
	args = append(args, idfPath)

	fmt.Printf("  → 命令: %s %s\n", exePath, strings.Join(args, " "))

	cmd := exec.Command(exePath, args...)

	// 实时打印 EnergyPlus 输出（前 20 行）
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("获取 stdout 失败: %w", err)
	}
	cmd.Stderr = cmd.Stdout // 合并 stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 EnergyPlus 失败: %w", err)
	}

	// 实时读取输出，只打印关键行
	scanner := bufio.NewScanner(stdout)
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		// 打印前 5 行（版本信息）和含关键词的行
		if lineCount <= 5 ||
			strings.Contains(line, "EnergyPlus Completed") ||
			strings.Contains(line, "Fatal") ||
			strings.Contains(line, "Severe") {
			fmt.Printf("    %s\n", line)
		}
	}

	_ = cmd.Wait()
	elapsed := time.Since(start)

	// 解析 .err 文件
	result := &SimResult{
		OutputDir: outputDir,
		Elapsed:   elapsed.Round(time.Millisecond),
	}
	result.ErrFile = findErrFile(outputDir)
	if result.ErrFile != "" {
		parseErrFile(result.ErrFile, result)
	}
	result.Success = len(result.Fatals) == 0

	return result, nil
}

// findErrFile 在输出目录中找 .err 文件
func findErrFile(dir string) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".err") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// parseErrFile 解析 EnergyPlus .err 文件，提取 Fatal/Severe/Warning 信息
func parseErrFile(errPath string, result *SimResult) {
	f, err := os.Open(errPath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "** fatal **"):
			result.Fatals = append(result.Fatals, strings.TrimSpace(line))
		case strings.Contains(lower, "** severe **"):
			result.Severes = append(result.Severes, strings.TrimSpace(line))
		case strings.Contains(lower, "** warning **"):
			result.Warnings = append(result.Warnings, strings.TrimSpace(line))
		}
	}
}

// ── 打印辅助 ─────────────────────────────────────────────────

func section(title string) {
	fmt.Println()
	fmt.Println("──────────────────────────────────────────────────")
	fmt.Printf("  %s\n", title)
	fmt.Println("──────────────────────────────────────────────────")
}

func success(msg string) { fmt.Printf("  ✓ %s\n", msg) }
func fail(msg string)    { fmt.Printf("  ✗ %s\n", msg) }

func printStrings(label string, items []string, max int) {
	if len(items) == 0 {
		return
	}
	fmt.Printf("  %s (%d):\n", label, len(items))
	for i, s := range items {
		if i >= max {
			fmt.Printf("    ... (共 %d 条，已截断)\n", len(items))
			break
		}
		fmt.Printf("    %s\n", s)
	}
}

// ── main ─────────────────────────────────────────────────────

func main() {
	idfPath := defaultIDF
	epwPath := defaultEPW
	if len(os.Args) > 1 {
		idfPath = os.Args[1]
	}
	if len(os.Args) > 2 {
		epwPath = os.Args[2]
	}

	// 以 IDF 文件名为基础建立输出子目录，避免覆盖其他结果
	baseName := strings.TrimSuffix(filepath.Base(idfPath), ".idf")
	outputDir := filepath.Join(defaultOutBase, baseName+"_"+time.Now().Format("20060102_150405"))

	fmt.Println("══════════════════════════════════════════════════")
	fmt.Println("  EnergyPlus RunSimulation Demo")
	fmt.Println("══════════════════════════════════════════════════")
	fmt.Printf("  IDF:        %s\n", idfPath)
	fmt.Printf("  EPW:        %s\n", epwPath)
	fmt.Printf("  输出目录:   %s\n", outputDir)

	// ── 1. 前置检查 ───────────────────────────────────────────
	section("1. 前置检查")

	for _, p := range []struct{ label, path string }{
		{"IDF 文件", idfPath},
		{"EPW 文件", epwPath},
	} {
		if _, err := os.Stat(p.path); err != nil {
			fail(fmt.Sprintf("%s 不存在: %s", p.label, p.path))
			os.Exit(1)
		}
		success(fmt.Sprintf("%s 存在", p.label))
	}

	exePath, err := findEnergyPlus()
	if err != nil {
		fail(err.Error())
		os.Exit(1)
	}
	success(fmt.Sprintf("EnergyPlus: %s", exePath))

	// ── 2. 运行仿真 ───────────────────────────────────────────
	section("2. 运行 EnergyPlus 仿真")

	result, err := RunSimulation(idfPath, epwPath, outputDir)
	if err != nil {
		fail(fmt.Sprintf("仿真启动失败: %v", err))
		os.Exit(1)
	}

	// ── 3. 结果摘要 ───────────────────────────────────────────
	section("3. 仿真结果")

	fmt.Printf("  耗时:       %v\n", result.Elapsed)
	fmt.Printf("  输出目录:   %s\n", result.OutputDir)
	if result.ErrFile != "" {
		fmt.Printf("  错误文件:   %s\n", result.ErrFile)
	}

	fmt.Println()
	if result.Success {
		success("仿真成功（无 Fatal 错误）")
	} else {
		fail(fmt.Sprintf("仿真失败（%d 个 Fatal 错误）", len(result.Fatals)))
	}

	printStrings("Fatal", result.Fatals, 5)
	printStrings("Severe", result.Severes, 5)
	if len(result.Warnings) > 0 {
		fmt.Printf("  Warnings:  %d 条\n", len(result.Warnings))
	}

	// ── 4. 列出输出文件 ──────────────────────────────────────
	section("4. 输出文件")
	entries, _ := os.ReadDir(result.OutputDir)
	for _, e := range entries {
		info, _ := e.Info()
		if info != nil {
			fmt.Printf("  %-40s  %6d KB\n", e.Name(), info.Size()/1024)
		}
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════")
	if result.Success {
		fmt.Println("  仿真完成 ✓")
	} else {
		fmt.Println("  仿真失败 ✗，请检查 .err 文件")
	}
	fmt.Println("══════════════════════════════════════════════════")

	if !result.Success {
		os.Exit(1)
	}
}
