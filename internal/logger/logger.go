// 日志模块
// 基于 Go 标准库 log/slog，同时输出到控制台（彩色）和文件（JSON 格式）。
// Debug 级别完整记录 LLM 思考过程、工具调用入参出参，方便问题排查。
// 全局 Logger 通过 Init() 初始化后，所有模块直接调用包级别函数使用。

package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	globalLogger *slog.Logger
	once         sync.Once
)

// colorHandler 带颜色的控制台 handler
type colorHandler struct {
	w     io.Writer
	level slog.Level
	mu    sync.Mutex
}

// ANSI 颜色码
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	return h
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	levelColor := colorGray
	levelStr := r.Level.String()
	switch r.Level {
	case slog.LevelDebug:
		levelColor = colorGray
		levelStr = "DBG"
	case slog.LevelInfo:
		levelColor = colorGreen
		levelStr = "INF"
	case slog.LevelWarn:
		levelColor = colorYellow
		levelStr = "WRN"
	case slog.LevelError:
		levelColor = colorRed
		levelStr = "ERR"
	}

	// 格式：时间 [级别] 消息  key=value ...
	time := r.Time.Format("15:04:05.000")
	fmt.Fprintf(h.w, "%s%s%s %s[%s]%s %s",
		colorGray, time, colorReset,
		levelColor, levelStr, colorReset,
		r.Message,
	)

	// 附加 key=value 属性
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(h.w, "  %s%s%s=%v", colorCyan, a.Key, colorReset, a.Value)
		return true
	})
	fmt.Fprintln(h.w)
	return nil
}

// Init 初始化全局 Logger
// level: "debug" | "info" | "warn" | "error"
// logFile: 日志文件路径（空字符串则不写文件）
// console: 是否同时输出到控制台
func Init(level, logFile string, console bool) error {
	var initErr error
	once.Do(func() {
		// 解析日志级别
		var slogLevel slog.Level
		switch strings.ToLower(level) {
		case "debug":
			slogLevel = slog.LevelDebug
		case "warn", "warning":
			slogLevel = slog.LevelWarn
		case "error":
			slogLevel = slog.LevelError
		default:
			slogLevel = slog.LevelInfo
		}

		var handlers []slog.Handler

		// 控制台 handler
		if console {
			handlers = append(handlers, &colorHandler{w: os.Stdout, level: slogLevel})
		}

		// 文件 handler（JSON 格式，方便机器解析）
		if logFile != "" {
			if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
				initErr = fmt.Errorf("创建日志目录失败: %w", err)
				return
			}
			f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				initErr = fmt.Errorf("打开日志文件失败 [%s]: %w", logFile, err)
				return
			}
			fileHandler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slogLevel})
			handlers = append(handlers, fileHandler)
		}

		if len(handlers) == 0 {
			// 保底：至少有一个 handler
			handlers = append(handlers, &colorHandler{w: os.Stdout, level: slogLevel})
		}

		globalLogger = slog.New(&multiHandler{handlers: handlers})
		slog.SetDefault(globalLogger)
	})
	return initErr
}

// multiHandler 将日志同时发送到多个 handler
type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: newHandlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: newHandlers}
}

// ============================================================
// 包级别便捷函数，所有模块直接调用
// ============================================================

func Get() *slog.Logger {
	if globalLogger == nil {
		// 未初始化时返回默认 logger
		return slog.Default()
	}
	return globalLogger
}

func Debug(msg string, args ...any) { Get().Debug(msg, args...) }
func Info(msg string, args ...any)  { Get().Info(msg, args...) }
func Warn(msg string, args ...any)  { Get().Warn(msg, args...) }
func Error(msg string, args ...any) { Get().Error(msg, args...) }

// Phase 打印阶段分隔线，突出显示当前流程阶段
func Phase(phase, msg string) {
	line := strings.Repeat("─", 60)
	fmt.Printf("\n%s%s%s\n", colorBold+colorBlue, line, colorReset)
	fmt.Printf("%s[PHASE] %-20s %s%s\n", colorBold+colorBlue, phase, msg, colorReset)
	fmt.Printf("%s%s%s\n\n", colorBold+colorBlue, line, colorReset)
	Get().Info("[PHASE] "+phase, "msg", msg)
}

// LLMThought 流式展示 LLM 思考内容（不换行，连续输出）
func LLMThought(token string) {
	fmt.Print(colorGray + token + colorReset)
}

// LLMThoughtEnd 思考结束换行
func LLMThoughtEnd() {
	fmt.Println()
}

// ToolCall 记录工具调用
func ToolCall(name string, args string) {
	fmt.Printf("\n%s[→ Tool]%s %s%s%s  %s%s%s\n",
		colorYellow+colorBold, colorReset,
		colorYellow, name, colorReset,
		colorGray, args, colorReset,
	)
	Get().Debug("[TOOL→]", "name", name, "args", args)
}

// ToolResult 记录工具返回
func ToolResult(name string, result string) {
	preview := result
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	fmt.Printf("%s[← Tool]%s %s%s%s  %s%s%s\n",
		colorCyan+colorBold, colorReset,
		colorCyan, name, colorReset,
		colorGray, preview, colorReset,
	)
	Get().Debug("[←TOOL]", "name", name, "result", preview)
}
