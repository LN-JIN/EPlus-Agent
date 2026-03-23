// UI 展示模块
// 提供命令行界面的格式化输出函数：分隔线、表格、摘要、YAML 内容展示等。
// 所有展示函数均直接写入 stdout，不经过日志系统（日志另存文件）。

package ui

import (
	"fmt"
	"os"
	"strings"

	"bufio"
)

// ============================================================
// 基础展示函数
// ============================================================

// PrintPhase 打印阶段标题横幅
func PrintPhase(phase, desc string) {
	line := strings.Repeat("═", 60)
	fmt.Printf("\n\033[1;34m%s\033[0m\n", line)
	fmt.Printf("\033[1;34m  %-20s  %s\033[0m\n", phase, desc)
	fmt.Printf("\033[1;34m%s\033[0m\n\n", line)
}

// PrintSection 打印小节标题
func PrintSection(title string) {
	fmt.Printf("\n\033[1;36m── %s %s\033[0m\n", title, strings.Repeat("─", max(0, 50-len(title))))
}

// PrintSuccess 打印成功消息
func PrintSuccess(msg string) {
	fmt.Printf("\033[32m✓ %s\033[0m\n", msg)
}

// PrintWarning 打印警告消息
func PrintWarning(msg string) {
	fmt.Printf("\033[33m⚠ %s\033[0m\n", msg)
}

// PrintError 打印错误消息
func PrintError(msg string) {
	fmt.Printf("\033[31m✗ %s\033[0m\n", msg)
}

// PrintInfo 打印普通信息
func PrintInfo(msg string) {
	fmt.Printf("  %s\n", msg)
}

// ============================================================
// 表格展示
// ============================================================

// PrintTable 打印两列 Markdown 风格表格
// title: 表格标题（空则不打印）
// rows: [][2]string，每行 [字段名, 值]
func PrintTable(title string, rows [][2]string) {
	if title != "" {
		PrintSection(title)
	}

	// 计算第一列最大宽度
	maxKey := 12
	for _, row := range rows {
		if len(row[0]) > maxKey {
			maxKey = len(row[0])
		}
	}

	// 表头
	fmt.Printf("  ┌%s┬%s┐\n",
		strings.Repeat("─", maxKey+2),
		strings.Repeat("─", 40))
	fmt.Printf("  │ %-*s │ %-38s │\n", maxKey, "字段", "值")
	fmt.Printf("  ├%s┼%s┤\n",
		strings.Repeat("─", maxKey+2),
		strings.Repeat("─", 40))

	for _, row := range rows {
		val := row[1]
		if val == "" {
			val = "\033[90m（未填写）\033[0m"
		}
		// 截断过长的值
		display := val
		if len(val) > 38 {
			display = val[:35] + "..."
		}
		fmt.Printf("  │ %-*s │ %-38s │\n", maxKey, row[0], display)
	}

	fmt.Printf("  └%s┴%s┘\n",
		strings.Repeat("─", maxKey+2),
		strings.Repeat("─", 40))
}

// ============================================================
// 意图摘要展示
// ============================================================

// PrintIntentSummary 展示 BuildingIntent 的完整摘要表格
// 接收 intent 字段的 map 形式（由 intent 层序列化提供，避免循环依赖）
func PrintIntentSummary(fields map[string]string) {
	PrintSection("建筑意图确认")
	rows := make([][2]string, 0, len(fields))
	// 按固定顺序展示关键字段
	orderedKeys := []string{
		"建筑名称", "建筑类型", "地理位置", "纬度/经度",
		"总面积(m²)", "楼层数", "热区数量",
		"外墙 U 值", "屋顶 U 值", "窗墙比（南）", "窗户 U 值/SHGC",
		"HVAC 系统", "制冷温度", "供暖温度",
		"使用类型", "工作日时间",
		"仿真周期",
	}
	seen := map[string]bool{}
	for _, k := range orderedKeys {
		if v, ok := fields[k]; ok {
			rows = append(rows, [2]string{k, v})
			seen[k] = true
		}
	}
	// 附加其余字段
	for k, v := range fields {
		if !seen[k] {
			rows = append(rows, [2]string{k, v})
		}
	}
	PrintTable("", rows)
}

// ============================================================
// YAML 内容展示
// ============================================================

// PrintYAMLContent 读取并展示 YAML 文件内容（前 N 行）
func PrintYAMLContent(path string, maxLines int) {
	PrintSection("生成的 YAML 配置（预览）")
	fmt.Printf("  文件路径: \033[1;32m%s\033[0m\n\n", path)

	f, err := os.Open(path)
	if err != nil {
		PrintError("无法读取文件: " + err.Error())
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		if maxLines > 0 && lineNum >= maxLines {
			fmt.Printf("\033[90m  ... （更多内容请查看文件）\033[0m\n")
			break
		}
		line := scanner.Text()
		// 简单语法高亮
		printYAMLLine(lineNum+1, line)
		lineNum++
	}
}

// printYAMLLine 简单的 YAML 行高亮输出
func printYAMLLine(n int, line string) {
	trimmed := strings.TrimSpace(line)

	switch {
	case strings.HasPrefix(trimmed, "#"):
		// 注释 - 灰色
		fmt.Printf("  \033[90m%4d │ %s\033[0m\n", n, line)
	case strings.HasPrefix(trimmed, "-"):
		// 列表项 - 普通颜色
		fmt.Printf("  \033[0m%4d │ %s\033[0m\n", n, line)
	case strings.Contains(line, ":") && !strings.HasPrefix(trimmed, "-"):
		// key: value 行 - key 用青色
		idx := strings.Index(line, ":")
		key := line[:idx]
		rest := line[idx:]
		fmt.Printf("  %4d │ \033[36m%s\033[0m%s\n", n, key, rest)
	default:
		fmt.Printf("  %4d │ %s\n", n, line)
	}
}

// PrintSummary 展示 MCP 验证摘要（带分隔线）
func PrintSummary(summary string) {
	PrintSection("MCP 配置摘要")
	lines := strings.Split(summary, "\n")
	for _, line := range lines {
		fmt.Printf("  %s\n", line)
	}
}

// PrintFinalResult 展示最终结果横幅
func PrintFinalResult(yamlPath, idfPath, simOutDir, reportPath, duration string) {
	line := strings.Repeat("═", 60)
	fmt.Printf("\n\033[1;32m%s\033[0m\n", line)
	fmt.Printf("\033[1;32m  ✓ EnergyPlus 所有流程已完成！\033[0m\n")
	fmt.Printf("\033[1;32m%s\033[0m\n", line)
	fmt.Println()
	if yamlPath != "" {
		fmt.Printf("  YAML 配置:  \033[1;33m%s\033[0m\n", yamlPath)
	}
	if idfPath != "" {
		fmt.Printf("  IDF 文件:   \033[1;33m%s\033[0m\n", idfPath)
	}
	if simOutDir != "" {
		fmt.Printf("  仿真输出:   \033[1;33m%s\033[0m\n", simOutDir)
	}
	if reportPath != "" {
		fmt.Printf("  分析报告:   \033[1;33m%s\033[0m\n", reportPath)
	}
	fmt.Printf("\n  总耗时:     %s\n\n", duration)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
