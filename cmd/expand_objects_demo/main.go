// ExpandObjects Demo
// 演示如何调用 EnergyPlus 的 ExpandObjects 工具将 HVACTemplate 对象展开为详细对象。
//
// ExpandObjects 的工作机制：
//   - 读取当前工作目录下的 in.idf
//   - 将 HVACTemplate:* 对象展开为完整的 EnergyPlus 对象
//   - 输出 expanded.idf（保留非模板对象，追加展开后的对象）
//
// 用法：
//
//	go run cmd/expand_objects_demo/main.go [idf路径]
//
// 若不传参数，使用默认 IDF 路径。
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ExpandObjects 可执行文件路径（自动检测，也可通过环境变量 EXPANDOBJECTS_EXE 覆盖）
func findExpandObjects() (string, error) {
	// 1. 环境变量优先
	if exe := os.Getenv("EXPANDOBJECTS_EXE"); exe != "" {
		return exe, nil
	}

	// 2. 尝试与 EnergyPlus 同目录
	if ep := os.Getenv("ENERGYPLUS_EXE"); ep != "" {
		dir := filepath.Dir(ep)
		candidate := filepath.Join(dir, expandObjectsBinary())
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// 3. 已知安装路径（Windows 常见位置）
	candidates := []string{
		`D:\programs\EnergyPlusV25-2-0\ExpandObjects.exe`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	// 4. PATH 查找
	if path, err := exec.LookPath(expandObjectsBinary()); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("找不到 ExpandObjects 可执行文件，请设置环境变量 EXPANDOBJECTS_EXE")
}

func expandObjectsBinary() string {
	if runtime.GOOS == "windows" {
		return "ExpandObjects.exe"
	}
	return "ExpandObjects"
}

// RunExpandObjects 在临时工作目录中运行 ExpandObjects
//
// idfPath:     输入的 IDF 文件路径
// outputDir:   输出目录（expanded.idf 将保存到此目录）
// 返回展开后的 IDF 文件路径
func RunExpandObjects(idfPath, outputDir string) (string, error) {
	exePath, err := findExpandObjects()
	if err != nil {
		return "", err
	}

	// ExpandObjects 只认当前目录下的 in.idf，所以需要临时工作目录
	workDir, err := os.MkdirTemp("", "expandobjects_*")
	if err != nil {
		return "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(workDir)

	// 将输入 IDF 复制为 in.idf
	inIDF := filepath.Join(workDir, "in.idf")
	if err := copyFile(idfPath, inIDF); err != nil {
		return "", fmt.Errorf("复制 IDF 失败: %w", err)
	}

	// 运行 ExpandObjects
	fmt.Printf("  → 运行: %s\n", exePath)
	fmt.Printf("  → 工作目录: %s\n", workDir)
	cmd := exec.Command(exePath)
	cmd.Dir = workDir

	var outBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if err := cmd.Run(); err != nil {
		// ExpandObjects 即使出错也可能返回非零退出码，检查输出
		output := outBuf.String()
		if strings.Contains(output, "Finished with Error") {
			return "", fmt.Errorf("ExpandObjects 执行出错:\n%s", output)
		}
		// 有时仅是警告，继续检查产物
		fmt.Printf("  ⚠ ExpandObjects 警告: %v\n", err)
	}

	fmt.Printf("  → ExpandObjects 输出:\n")
	for _, line := range strings.Split(strings.TrimSpace(outBuf.String()), "\n") {
		fmt.Printf("      %s\n", line)
	}

	// 检查展开结果
	expandedSrc := filepath.Join(workDir, "expanded.idf")
	if _, err := os.Stat(expandedSrc); err != nil {
		// 若无 HVACTemplate，ExpandObjects 可能不生成 expanded.idf，直接用 in.idf
		fmt.Println("  ℹ expanded.idf 未生成（IDF 中可能没有 HVACTemplate 对象），使用原始 IDF")
		expandedSrc = inIDF
	}

	// 将结果复制到输出目录
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("创建输出目录失败: %w", err)
	}
	baseName := strings.TrimSuffix(filepath.Base(idfPath), ".idf")
	outPath := filepath.Join(outputDir, baseName+"_expanded.idf")
	if err := copyFile(expandedSrc, outPath); err != nil {
		return "", fmt.Errorf("保存展开结果失败: %w", err)
	}

	return outPath, nil
}

// copyFile 复制单个文件
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// countHVACTemplates 统计 IDF 中 HVACTemplate 对象的数量
func countHVACTemplates(idfPath string) (int, error) {
	data, err := os.ReadFile(idfPath)
	if err != nil {
		return 0, err
	}
	count := strings.Count(strings.ToLower(string(data)), "hvactemplate:")
	return count, nil
}

// compareIDF 简单比较两个 IDF 的行数差异
func compareIDF(original, expanded string) {
	origData, _ := os.ReadFile(original)
	expData, _ := os.ReadFile(expanded)

	origLines := len(strings.Split(string(origData), "\n"))
	expLines := len(strings.Split(string(expData), "\n"))

	fmt.Printf("  原始 IDF:  %d 行\n", origLines)
	fmt.Printf("  展开 IDF:  %d 行\n", expLines)
	if expLines > origLines {
		fmt.Printf("  新增行数:  +%d 行（HVACTemplate 已展开为完整对象）\n", expLines-origLines)
	} else {
		fmt.Printf("  行数无变化（无 HVACTemplate 对象被展开）\n")
	}
}

// ── 辅助打印 ────────────────────────────────────────────────

func section(title string) {
	fmt.Println()
	fmt.Println("──────────────────────────────────────────────────")
	fmt.Printf("  %s\n", title)
	fmt.Println("──────────────────────────────────────────────────")
}

func success(msg string) { fmt.Printf("  ✓ %s\n", msg) }
func fail(msg string)    { fmt.Printf("  ✗ %s\n", msg) }

// ── main ─────────────────────────────────────────────────────

func main() {
	// 默认 IDF 路径（可通过命令行参数覆盖）
	idfPath := `D:\TryAgent\energyplus-agent\output\simulation\temp_20260320_143304.idf`
	if len(os.Args) > 1 {
		idfPath = os.Args[1]
	}

	outputDir := filepath.Join(filepath.Dir(idfPath), "expand_output")

	fmt.Println("══════════════════════════════════════════════════")
	fmt.Println("  EnergyPlus ExpandObjects Demo")
	fmt.Println("══════════════════════════════════════════════════")
	fmt.Printf("  IDF 输入:  %s\n", idfPath)
	fmt.Printf("  输出目录:  %s\n", outputDir)

	// ── 1. 检查输入文件 ───────────────────────────────────────
	section("1. 检查输入 IDF")
	if _, err := os.Stat(idfPath); err != nil {
		fail(fmt.Sprintf("IDF 文件不存在: %v", err))
		os.Exit(1)
	}
	success("IDF 文件存在")

	templateCount, err := countHVACTemplates(idfPath)
	if err != nil {
		fail(fmt.Sprintf("读取 IDF 失败: %v", err))
		os.Exit(1)
	}
	if templateCount > 0 {
		fmt.Printf("  ℹ 检测到 %d 个 HVACTemplate 对象，将被展开\n", templateCount)
	} else {
		fmt.Println("  ℹ 未检测到 HVACTemplate 对象（ExpandObjects 将原样输出）")
	}

	// ── 2. 检测 ExpandObjects 可执行文件 ─────────────────────
	section("2. 检测 ExpandObjects")
	exePath, err := findExpandObjects()
	if err != nil {
		fail(err.Error())
		os.Exit(1)
	}
	success(fmt.Sprintf("找到 ExpandObjects: %s", exePath))

	// ── 3. 运行 ExpandObjects ────────────────────────────────
	section("3. 运行 ExpandObjects")
	startTime := time.Now()

	expandedPath, err := RunExpandObjects(idfPath, outputDir)
	if err != nil {
		fail(fmt.Sprintf("ExpandObjects 失败: %v", err))
		os.Exit(1)
	}

	elapsed := time.Since(startTime)
	success(fmt.Sprintf("ExpandObjects 完成，耗时 %v", elapsed.Round(time.Millisecond)))
	fmt.Printf("  → 展开结果: %s\n", expandedPath)

	// ── 4. 对比结果 ──────────────────────────────────────────
	section("4. 结果对比")
	compareIDF(idfPath, expandedPath)

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════")
	fmt.Println("  Demo 完成")
	fmt.Println("══════════════════════════════════════════════════")
}
