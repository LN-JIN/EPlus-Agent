// 用户输入处理模块
// 提供统一的命令行输入接口，包括单行输入、多行输入和确认交互。
// 支持用户在意图确认环节输入修改意见或取消操作。

package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var stdinReader = bufio.NewReader(os.Stdin)

// ReadLine 打印提示符后读取用户输入的一行文本
func ReadLine(prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := stdinReader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// ReadMultiLine 读取多行输入，直到用户输入单独的 "." 结束
// 用于收集用户较长的建筑描述
func ReadMultiLine(prompt string) (string, error) {
	fmt.Println(prompt)
	fmt.Println("（输入完成后，单独输入一行 \".\" 结束）")
	var lines []string
	for {
		fmt.Print("  > ")
		line, err := stdinReader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "." {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

// ConfirmResult 意图确认的结果类型
type ConfirmResult int

const (
	ConfirmYes      ConfirmResult = iota // 用户确认，直接继续
	ConfirmModify                        // 用户有修改意见，附带修改内容
	ConfirmCancel                        // 用户取消整个流程
)

// ReadConfirm 展示意图表格后请求用户确认
// 返回确认结果和可选的修改意见文本
// 交互规则：
//   - 直接按 Enter → 确认
//   - 输入修改意见文字 → 返回 ConfirmModify
//   - 输入 "q" 或 "quit" → 取消
func ReadConfirm(prompt string) (ConfirmResult, string) {
	fmt.Printf("\n%s\n", prompt)
	fmt.Println("  [Enter]  确认，继续生成 YAML")
	fmt.Println("  [文字]   输入修改意见，重新收集")
	fmt.Println("  [q]      取消并退出")
	fmt.Print("\n  > ")

	line, err := stdinReader.ReadString('\n')
	if err != nil {
		return ConfirmCancel, ""
	}
	line = strings.TrimRight(line, "\r\n")
	line = strings.TrimSpace(line)

	switch strings.ToLower(line) {
	case "", "y", "yes", "确认", "ok":
		return ConfirmYes, ""
	case "q", "quit", "exit", "取消", "退出":
		return ConfirmCancel, ""
	default:
		return ConfirmModify, line
	}
}

// AskQuestion 打印问题并等待用户回答（用于 ask_user 工具的实现）
func AskQuestion(question string) (string, error) {
	fmt.Printf("\n\033[1;33m[Agent 提问]\033[0m %s\n", question)
	return ReadLine("  您的回答: ")
}
