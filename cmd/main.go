// EnergyPlus Agent 入口程序
// 启动流程：加载配置 → 初始化日志 → 读取用户描述 → 运行 Orchestrator。
// 支持通过命令行参数指定配置文件路径，默认使用 configs/config.yaml。
// 所有错误均打印到 stderr 并以非零状态码退出，方便脚本集成。

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/logger"
	"energyplus-agent/internal/orchestrator"
	"energyplus-agent/internal/ui"
)

func main() {
	// ── 命令行参数 ──────────────────────────────────────────────
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	inputText := flag.String("input", "", "建筑描述（可选，不填则进入交互模式）")
	flag.Parse()

	// ── 加载配置 ──────────────────────────────────────────────
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：加载配置失败: %v\n", err)
		fmt.Fprintf(os.Stderr, "提示：请确认 %s 文件存在，或使用 -config 指定正确路径\n", *configPath)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "错误：配置校验失败: %v\n", err)
		fmt.Fprintf(os.Stderr, "提示：可通过设置环境变量 LLM_API_KEY=sk-xxx 来提供 API Key\n")
		os.Exit(1)
	}

	// ── 初始化日志 ────────────────────────────────────────────
	if err := logger.Init(cfg.Log.Level, cfg.Log.File, cfg.Log.Console); err != nil {
		fmt.Fprintf(os.Stderr, "警告：日志初始化失败（继续运行）: %v\n", err)
	}

	// ── 打印启动横幅 ──────────────────────────────────────────
	printBanner(cfg)

	// ── 读取用户输入 ──────────────────────────────────────────
	userInput := *inputText
	if userInput == "" {
		userInput, err = readUserInput()
		if err != nil || userInput == "" {
			fmt.Fprintln(os.Stderr, "错误：未获取到建筑描述")
			os.Exit(1)
		}
	}

	// ── 设置 Context（支持 Ctrl+C 优雅退出）────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── 运行主流程 ───────────────────────────────────────────
	orch := orchestrator.New(cfg)
	if err := orch.Run(ctx, userInput); err != nil {
		// 区分用户中断和系统错误
		if ctx.Err() != nil {
			fmt.Println("\n\n已中断（Ctrl+C）")
			os.Exit(0)
		}
		ui.PrintError(fmt.Sprintf("运行失败: %v", err))
		logger.Error("[Main] 流程异常退出", "err", err)
		os.Exit(1)
	}
}

// readUserInput 交互式读取用户的建筑描述
func readUserInput() (string, error) {
	ui.PrintSection("请描述您的建筑")
	fmt.Println("\n  请用自然语言描述您要模拟的建筑，例如：")
	fmt.Println("    \"深圳市一栋 3 层住宅，建筑面积约 400 平方米，南北朝向\"")
	fmt.Println("    \"北京市某办公楼，6 层，每层 500 平方米，东西向\"")
	fmt.Println()
	fmt.Println("  您可以输入多行，完成后单独输入 \".\" 结束：")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	var lines []string

	for {
		fmt.Print("  > ")
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "." {
			break
		}
		if line != "" {
			lines = append(lines, line)
		}
		// 如果输入了一行非空内容且下一行可能是 "."，继续等待
	}

	result := strings.Join(lines, "\n")
	result = strings.TrimSpace(result)
	return result, nil
}

// printBanner 打印启动横幅
func printBanner(cfg *config.Config) {
	fmt.Println()
	fmt.Println("\033[1;36m╔══════════════════════════════════════════════════════════╗\033[0m")
	fmt.Println("\033[1;36m║          EnergyPlus Agent  v0.1 (Go)                    ║\033[0m")
	fmt.Println("\033[1;36m║     意图收集 → YAML 生成 → MCP 转换 → 展示 IDF          ║\033[0m")
	fmt.Println("\033[1;36m╚══════════════════════════════════════════════════════════╝\033[0m")
	fmt.Println()
	fmt.Printf("  模型:    \033[33m%s\033[0m  (%s)\n", cfg.LLM.Model, cfg.LLM.BaseURL)
	fmt.Printf("  MCP:     \033[33m%s\033[0m\n", cfg.MCP.BaseURL)
	fmt.Printf("  输出目录: \033[33m%s\033[0m\n", cfg.Session.OutputDir)
	fmt.Printf("  日志文件: \033[33m%s\033[0m\n", cfg.Log.File)
	fmt.Println()
}
