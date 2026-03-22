// Skill 模块：解析 SKILL.md 文件，提取元数据和使用指令。
// 参考 Python 项目的 MarkdownSkill 设计，每个 skill 目录包含：
//   - SKILL.md：YAML frontmatter（元数据）+ Markdown 正文（使用指令）
//   - references/：参考数据文件目录（由 LLM 按需查阅）

package skills

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Skill 表示一个已解析的 skill
type Skill struct {
	Name          string // skill 唯一标识符
	Description   string // skill 功能描述（用于 System Prompt 说明）
	Phase         string // 适用阶段：intent / simulation / report / any
	ReferencesDir string // references 目录路径（相对于项目根目录）
	Instructions  string // SKILL.md 正文（使用指令，注入 System Prompt）
}

// ParseSkillFile 解析 SKILL.md 文件，返回 Skill 结构体
func ParseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 skill 文件失败 [%s]: %w", path, err)
	}

	content := string(data)
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("解析 frontmatter 失败 [%s]: %w", path, err)
	}

	skill := &Skill{
		Instructions: strings.TrimSpace(body),
	}

	// 解析 frontmatter 的 key: value 对
	scanner := bufio.NewScanner(strings.NewReader(frontmatter))
	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "name":
			skill.Name = val
		case "description":
			skill.Description = val
		case "phase":
			skill.Phase = val
		case "references_dir":
			skill.ReferencesDir = val
		}
	}

	if skill.Name == "" {
		return nil, fmt.Errorf("skill 文件缺少 name 字段 [%s]", path)
	}

	return skill, nil
}

// splitFrontmatter 将 "---\n...\n---\n body" 格式分割为 frontmatter 和 body
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	if !strings.HasPrefix(content, "---") {
		return "", content, nil // 无 frontmatter，整体为 body
	}

	// 找到第一个 --- 之后的第二个 ---
	rest := content[3:] // 跳过开头的 ---
	// 跳过紧跟的换行
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return "", content, fmt.Errorf("frontmatter 未闭合（缺少结尾 ---）")
	}

	frontmatter = rest[:endIdx]
	body = rest[endIdx+4:] // 跳过 \n---
	// 跳过紧跟结尾 --- 的换行
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	} else if len(body) > 1 && body[0] == '\r' && body[1] == '\n' {
		body = body[2:]
	}

	return frontmatter, body, nil
}
