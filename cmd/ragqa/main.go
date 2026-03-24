// cmd/ragqa/main.go — EnergyPlus IOR RAG 问答工具
// 独立 CLI，不依赖 EPlus-Agent 主流程，直接加载向量索引并启动交互式问答。
//
// 用法:
//
//	go run cmd/ragqa/main.go [--config configs/config.yaml] [--index data/index/ior_part.idx] [--no-hyde]
//
// 追问说明:
//
//	输入短句或含指代词（它/这/该/那）的问题时，自动复用上轮检索结果，跳过重复检索。
//	输入 "!new" 前缀（如 "!new Zone 对象的字段有哪些？"）可强制重新检索。
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"energyplus-agent/internal/config"
	"energyplus-agent/internal/llm"
	"energyplus-agent/internal/rag"
	"energyplus-agent/internal/rag/vectorstore"
)

// followUpWords 追问常见指代词/连接词（出现即可能是追问）
var followUpWords = []string{
	"它", "这", "该", "那", "此", "其",
	"上述", "前面", "刚才", "之前", "前边",
	"还有", "另外", "继续", "同样", "再问",
	"那么", "如果是", "如果这", "那如果",
}

// isFollowUp 判断是否为追问
// 短句（≤15个字）或含指代词，且上轮有检索结果，则判定为追问
func isFollowUp(query string, hasLastContext bool) bool {
	if !hasLastContext {
		return false
	}
	// 强制新检索前缀
	if strings.HasPrefix(query, "!new") {
		return false
	}
	// 短句视为追问
	if utf8.RuneCountInString(query) <= 15 {
		return true
	}
	// 含指代词视为追问
	for _, w := range followUpWords {
		if strings.Contains(query, w) {
			return true
		}
	}
	return false
}

func main() {
	// ── 参数解析 ─────────────────────────────────────────────────────────────
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	indexPath := flag.String("index", "", "向量索引文件路径（覆盖 config.yaml 中的 rag.index_path）")
	noHyDE := flag.Bool("no-hyde", false, "禁用 HyDE 双路检索（仅用直接 embedding）")
	logFile := flag.String("log", "logs/ragqa.log", "完整会话日志文件路径（含思考过程）")
	flag.Parse()

	// ── 加载配置 ─────────────────────────────────────────────────────────────
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "配置校验失败: %v\n", err)
		os.Exit(1)
	}

	if *indexPath != "" {
		cfg.RAG.IndexPath = *indexPath
	}
	useHyDE := cfg.RAG.HyDEEnabled && !*noHyDE

	// ── slog（仅 Warn 以上输出到 stderr）────────────────────────────────────
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})))

	// ── 日志文件 ──────────────────────────────────────────────────────────────
	logger, closeLog, err := openLogger(*logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法打开日志文件 %s: %v\n", *logFile, err)
		logger = nil
	} else {
		defer closeLog()
	}

	// ── 加载向量索引 ─────────────────────────────────────────────────────────
	fmt.Printf("加载索引: %s\n", cfg.RAG.IndexPath)
	store, err := vectorstore.LoadFromFile(cfg.RAG.IndexPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载索引失败: %v\n", err)
		fmt.Fprintf(os.Stderr, "请先运行: python scripts/build_rag_index.py\n")
		os.Exit(1)
	}
	fmt.Printf("索引加载完成: %d parent chunks, %d child chunks\n",
		store.NumParents(), store.Len())

	// ── 初始化组件 ────────────────────────────────────────────────────────────
	embedder := rag.NewEmbeddingAdapter(
		cfg.LLM.BaseURL, cfg.LLM.APIKey,
		cfg.RAG.EmbeddingModel, cfg.RAG.EmbeddingDim, cfg.RAG.EmbeddingTimeoutSec,
	)
	llmClient := llm.NewClient(
		cfg.LLM.BaseURL, cfg.LLM.APIKey,
		cfg.LLM.Model, cfg.LLM.TimeoutSec, cfg.LLM.Temperature,
	)
	engine := rag.NewQAEngine(store, embedder, llmClient, cfg.RAG.TopK, useHyDE)

	// ── 启动信息 ──────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║     EnergyPlus IOR RAG 问答工具                         ║")
	fmt.Println("║     输入问题后按 Enter，输入 'exit' 或 Ctrl+C 退出       ║")
	fmt.Println("║     追问自动复用检索结果；用 !new 前缀强制重新检索       ║")
	fmt.Printf("║     LLM: %-46s║\n", cfg.LLM.Model)
	fmt.Printf("║     Embedding: %-42s║\n", cfg.RAG.EmbeddingModel)
	fmt.Printf("║     HyDE: %-47s║\n", boolStr(useHyDE))
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	if logger != nil {
		fmt.Printf("日志: %s\n", *logFile)
	}
	fmt.Println()

	logWrite(logger, "=== 会话开始 %s ===\n模型: %s | Embedding: %s | HyDE: %s\n\n",
		time.Now().Format("2006-01-02 15:04:05"),
		cfg.LLM.Model, cfg.RAG.EmbeddingModel, boolStr(useHyDE))

	// ── 多轮状态 ──────────────────────────────────────────────────────────────
	var history []llm.Message          // user/assistant 交替的对话历史
	var lastCtx *rag.RetrievedContext  // 上一轮检索结果（追问复用）
	const maxHistory = 10              // 最多保留 10 轮（20 条消息）

	// ── 交互循环 ─────────────────────────────────────────────────────────────
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("❓ 问题: ")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" {
			continue
		}
		if query == "exit" || query == "quit" || query == "q" {
			fmt.Println("再见！")
			break
		}

		// 处理 !new 前缀
		forceNew := strings.HasPrefix(query, "!new")
		if forceNew {
			query = strings.TrimSpace(strings.TrimPrefix(query, "!new"))
			if query == "" {
				continue
			}
		}

		// 判断是否追问
		followUp := !forceNew && isFollowUp(query, lastCtx != nil)

		logWrite(logger, "────────────────────────────────────────\n[%s] 问题: %s",
			time.Now().Format("15:04:05"), query)
		if followUp {
			logWrite(logger, "  [追问，复用检索结果]")
		}
		logWrite(logger, "\n\n")

		start := time.Now()
		ctx := context.Background()

		// 思考过程收集
		var thinkingBuf strings.Builder
		thinkingStarted := false
		mu := sync.Mutex{}

		onThinking := func(token string) {
			mu.Lock()
			defer mu.Unlock()
			if !thinkingStarted {
				fmt.Println()
				fmt.Println("💭 思考中:")
				fmt.Println(strings.Repeat("·", 60))
				thinkingStarted = true
			}
			fmt.Print(token)
			thinkingBuf.WriteString(token)
		}

		// 回答开始时打印分隔线（仅一次）
		answerHeaderPrinted := false
		mu2 := sync.Mutex{}

		fmt.Println()

		// 构建选项
		opts := &rag.AnswerOptions{
			History: history,
		}
		if followUp {
			opts.ReuseContext = lastCtx
		}

		result, err := engine.Answer(ctx, query, func(token string) {
			mu.Lock()
			wasThinking := thinkingStarted
			if thinkingStarted {
				thinkingStarted = false
			}
			mu.Unlock()

			mu2.Lock()
			if !answerHeaderPrinted {
				answerHeaderPrinted = true
				mu2.Unlock()
				if wasThinking {
					fmt.Println()
					fmt.Println(strings.Repeat("·", 60))
					fmt.Println()
				}
				fmt.Println("💬 回答:")
				fmt.Println(strings.Repeat("─", 60))
			} else {
				mu2.Unlock()
			}
			fmt.Print(token)
		}, onThinking, opts)

		fmt.Println()
		elapsed := time.Since(start)

		if err != nil {
			fmt.Fprintf(os.Stderr, "\n[错误] %v\n", err)
			logWrite(logger, "[错误] %v\n\n", err)
		}

		// 显示来源
		fmt.Println(strings.Repeat("─", 60))
		if result != nil && len(result.Parents) > 0 {
			if result.SkippedRetrieval {
				fmt.Printf("📚 参考来源（复用上轮，%d 个）:\n", len(result.Parents))
			} else {
				fmt.Printf("📚 参考来源（%d 个）:\n", len(result.Parents))
			}
			for i, p := range result.Parents {
				fmt.Printf("  [%d] %s，第 %d 页\n", i+1, p.IDDObject, p.PageStart)
			}
		}
		fmt.Printf("⏱  耗时: %.1fs\n", elapsed.Seconds())
		fmt.Println()

		// 写日志
		if result != nil {
			if thinkingBuf.Len() > 0 {
				logWrite(logger, "[思考过程]\n%s\n\n", thinkingBuf.String())
			}
			logWrite(logger, "[回答]\n%s\n\n", result.AnswerContent)
			if len(result.Parents) > 0 {
				logWrite(logger, "[参考来源]\n")
				for i, p := range result.Parents {
					logWrite(logger, "  [%d] %s，第 %d 页\n", i+1, p.IDDObject, p.PageStart)
				}
				logWrite(logger, "\n")
			}
		}
		logWrite(logger, "[耗时] %.1fs\n\n", elapsed.Seconds())

		// 更新多轮状态
		if result != nil {
			// 更新检索缓存（强制新检索时更新；追问不更新，保留上轮）
			if !result.SkippedRetrieval {
				lastCtx = result.RetrievedContext
			}
			// 追加对话历史（只保留最近 maxHistory 轮）
			history = append(history,
				llm.Message{Role: llm.RoleUser, Content: query},
				llm.Message{Role: llm.RoleAssistant, Content: result.AnswerContent},
			)
			if len(history) > maxHistory*2 {
				history = history[len(history)-maxHistory*2:]
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "读取输入错误: %v\n", err)
		os.Exit(1)
	}
	logWrite(logger, "=== 会话结束 %s ===\n", time.Now().Format("2006-01-02 15:04:05"))
}

func openLogger(path string) (*os.File, func(), error) {
	if err := os.MkdirAll("logs", 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}

func logWrite(f *os.File, format string, args ...any) {
	if f == nil {
		return
	}
	fmt.Fprintf(f, format, args...)
}

func boolStr(b bool) string {
	if b {
		return "开启"
	}
	return "关闭"
}
