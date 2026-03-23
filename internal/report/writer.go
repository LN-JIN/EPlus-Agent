// Markdown 报告写入工具

package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Section 报告章节
type Section struct {
	Title   string
	Content string
}

// WriteReport 将报告内容写入 Markdown 文件
// path: 输出文件路径（自动创建父目录）
// title: 报告标题
// sections: 按顺序排列的章节列表
func WriteReport(path, title string, sections []Section) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建报告目录失败: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	sb.WriteString(fmt.Sprintf("*Generated: %s*\n\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("---\n\n")

	for _, s := range sections {
		sb.WriteString(fmt.Sprintf("## %s\n\n", s.Title))
		sb.WriteString(s.Content)
		if !strings.HasSuffix(s.Content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
