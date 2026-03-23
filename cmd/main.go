// EnergyPlus Agent 入口程序
// 启动流程：加载配置 → 初始化日志 → 读取用户描述 → 运行 Orchestrator。
//
// 灵活入口支持：
//   -input    从建筑描述开始完整 6 阶段流程（默认交互模式）
//   -yaml     已有 YAML，跳过阶段 1/2，从阶段 3（YAML→IDF）开始
//   -idf      已有 IDF，跳过阶段 1-3，从阶段 4（仿真）开始
//   -sim-dir  已有仿真目录，跳过阶段 1-4，从阶段 5（报告）开始
//   -epw      自定义 EPW 气象文件（覆盖配置文件默认值）
//   -resume   续传会话 ID（从上次断点继续）
//   -skip-report  跳过阶段 5 报告解读
//   -skip-param   跳过阶段 6 参数分析

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
	configPath  := flag.String("config",       "configs/config.yaml", "配置文件路径")
	inputText   := flag.String("input",        "", "建筑描述（可选，不填则进入交互模式）")
	yamlPath    := flag.String("yaml",         "", "直接提供 YAML 路径，跳过阶段 1/2")
	idfPath     := flag.String("idf",          "", "直接提供 IDF 路径，跳过阶段 1-3")
	simDir      := flag.String("sim-dir",      "", "直接提供仿真输出目录，跳过阶段 1-4")
	epwPath     := flag.String("epw",          "", "自定义 EPW 气象文件路径（覆盖配置默认值）")
	resumeID    := flag.String("resume",       "", "续传会话 ID（从已有 session JSON 恢复）")
	skipReport   := flag.Bool("skip-report",     false, "跳过阶段 5（报告解读）")
	skipParam    := flag.Bool("skip-param",      false, "跳过阶段 6（参数分析）")
	analysisGoal := flag.String("analysis-goal", "",    "Phase 6 参数分析目标（空字符串则交互询问）")
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

	// ── 构建 RunConfig ────────────────────────────────────────
	runCfg := orchestrator.RunConfig{
		YAMLPath:     *yamlPath,
		IDFPath:      *idfPath,
		SimOutDir:    *simDir,
		EPWPath:      *epwPath,
		ResumeID:     *resumeID,
		SkipReport:   *skipReport,
		SkipParam:    *skipParam,
		AnalysisGoal: *analysisGoal,
	}

	// 确定用户输入（只在从头开始时需要）
	if *idfPath == "" && *yamlPath == "" && *simDir == "" && *resumeID == "" {
		userInput := *inputText
		if userInput == "" {
			userInput, err = readUserInput()
			if err != nil || userInput == "" {
				fmt.Fprintln(os.Stderr, "错误：未获取到建筑描述")
				os.Exit(1)
			}
		}
		runCfg.UserInput = userInput
	}

	// ── 设置 Context（支持 Ctrl+C 优雅退出）────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── 运行主流程 ───────────────────────────────────────────
	orch := orchestrator.New(cfg)
	if err := orch.RunWithConfig(ctx, runCfg); err != nil {
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
	}

	result := strings.Join(lines, "\n")
	result = strings.TrimSpace(result)
	return result, nil
}

// printBanner 打印启动横幅
func printBanner(cfg *config.Config) {
	fmt.Println()
	fmt.Println("\033[1;36m╔══════════════════════════════════════════════════════════╗\033[0m")
	fmt.Println("\033[1;36m║       EnergyPlus Agent  v0.2 (Go)                       ║\033[0m")
	fmt.Println("\033[1;36m║  意图收集→YAML生成→IDF转换→仿真→报告→参数分析            ║\033[0m")
	fmt.Println("\033[1;36m╚══════════════════════════════════════════════════════════╝\033[0m")
	fmt.Println()
	fmt.Printf("  模型:    \033[33m%s\033[0m  (%s)\n", cfg.LLM.Model, cfg.LLM.BaseURL)
	fmt.Printf("  MCP:     \033[33m%s\033[0m\n", cfg.MCP.BaseURL)
	fmt.Printf("  脚本:    \033[33m%s\033[0m\n", cfg.Session.SimulationScript)
	fmt.Printf("  输出目录: \033[33m%s\033[0m\n", cfg.Session.OutputDir)
	fmt.Printf("  日志文件: \033[33m%s\033[0m\n", cfg.Log.File)
	fmt.Println()
}
