// SameErrorGuard 检测重试循环中连续出现相同错误（"空转"），
// 防止 LLM 对无法修复的错误反复做无效尝试。
//
// 典型使用场景：
//   - YAML→IDF 转换：相同 Schema 错误连续 2 次 → 停止
//   - EnergyPlus 仿真：相同 Fatal 错误连续 3 次 → 停止（中间给 LLM 一次提示）

package fault

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// SameErrorGuard 连续相同错误检测器
type SameErrorGuard struct {
	// MaxRepeat 允许相同错误连续出现的次数上限，达到则返回 spinning=true。
	// 建议：非 ReAct 循环（idfconvert/yaml）用 2，ReAct 循环（simulation）用 3。
	MaxRepeat int

	lastNorm string
	count    int
}

// Observe 记录本次错误消息并返回：
//   spinning = true  → 相同错误已连续出现 MaxRepeat 次，调用方应终止循环。
//   hint            → 若非空，建议注入到下一次 LLM 修复提示中（停止前最后一次提醒）。
func (g *SameErrorGuard) Observe(errMsg string) (spinning bool, hint string) {
	if g.MaxRepeat <= 0 {
		g.MaxRepeat = 2
	}

	norm := normalizeErrorMsg(errMsg)
	if norm == g.lastNorm {
		g.count++
	} else {
		g.lastNorm = norm
		g.count = 1
	}

	if g.count >= g.MaxRepeat {
		return true, fmt.Sprintf(
			"已连续 %d 次出现相同错误，修复循环终止（错误指纹: %s）。",
			g.count, errFingerprint(norm),
		)
	}

	// 在停止前一步给出警告（仅当 MaxRepeat > 1 时才有意义）
	if g.MaxRepeat > 1 && g.count == g.MaxRepeat-1 {
		return false, fmt.Sprintf(
			"警告：已 %d 次出现完全相同的错误，上次修复未能改变错误内容。"+
				"请尝试完全不同的修复策略，否则下次相同错误将停止重试。",
			g.count,
		)
	}

	return false, ""
}

// Reset 重置状态（切换到新的重试序列时调用）。
func (g *SameErrorGuard) Reset() {
	g.lastNorm = ""
	g.count = 0
}

// ── 内部辅助 ──────────────────────────────────────────────────────────────────

var (
	reWinPath = regexp.MustCompile(`[A-Za-z]:\\[^\s]+`)
	reUnixPath = regexp.MustCompile(`(?:^|[\s(])/[^\s)]+`)
	reNumbers  = regexp.MustCompile(`\b\d{6,}\b`)         // 6+ 位纯数字（会话 ID 等）
	reLineCol  = regexp.MustCompile(`\b(?:line|column)\s+\d+`)
)

// normalizeErrorMsg 去除错误消息中的变量部分（路径、行号、大数字），
// 使相同类型的错误产生相同指纹。
func normalizeErrorMsg(msg string) string {
	msg = reWinPath.ReplaceAllString(msg, "<PATH>")
	msg = reUnixPath.ReplaceAllString(msg, " <PATH>")
	msg = reNumbers.ReplaceAllString(msg, "<N>")
	msg = reLineCol.ReplaceAllString(msg, "line N")
	return strings.ToLower(strings.TrimSpace(msg))
}

// errFingerprint 返回错误的短 hash（用于日志显示）。
func errFingerprint(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", sum[:3]) // 6 位 hex
}
