// SkillLoader：扫描 skills/ 目录，加载所有 SKILL.md 文件，按 phase 分组。
// 启动时仅读取 SKILL.md（轻量），不预加载 references/ 目录内容。
// references/ 内容由 LLM 通过工具调用按需读取。

package skills

import (
	"log/slog"
	"os"
	"path/filepath"
)

// Loader 管理所有已加载的 skill
type Loader struct {
	skills map[string]*Skill // name → Skill
}

// Load 扫描 skillsDir 目录，加载所有子目录中的 SKILL.md
// skillsDir 示例："skills"
func Load(skillsDir string) *Loader {
	loader := &Loader{
		skills: make(map[string]*Skill),
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		slog.Warn("[Skills] 无法读取 skills 目录", "dir", skillsDir, "err", err)
		return loader
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		skill, err := ParseSkillFile(skillFile)
		if err != nil {
			slog.Warn("[Skills] 跳过无效的 skill", "file", skillFile, "err", err)
			continue
		}
		loader.skills[skill.Name] = skill
		slog.Info("[Skills] 已加载 skill", "name", skill.Name, "phase", skill.Phase)
	}

	return loader
}

// ByPhase 返回指定阶段的所有 skill（phase="any" 也包含在任意阶段中）
func (l *Loader) ByPhase(phase string) []*Skill {
	var result []*Skill
	for _, s := range l.skills {
		if s.Phase == phase || s.Phase == "any" {
			result = append(result, s)
		}
	}
	return result
}

// All 返回所有已加载的 skill
func (l *Loader) All() []*Skill {
	result := make([]*Skill, 0, len(l.skills))
	for _, s := range l.skills {
		result = append(result, s)
	}
	return result
}

// BuildPromptSection 将指定阶段的 skill 指令拼合为 System Prompt 注入文本
// 返回格式：
//
//	## 可用规范查询工具（来自 Skills）
//	### query_standard
//	<instructions>
func (l *Loader) BuildPromptSection(phase string) string {
	skills := l.ByPhase(phase)
	if len(skills) == 0 {
		return ""
	}

	var sb string
	sb += "\n\n---\n\n## 规范查询（Skills）\n\n"
	sb += "以下 skill 提供了规范数据查询能力。当你对参数标准值不确定时，使用对应工具查阅。\n\n"

	for _, s := range skills {
		sb += "### " + s.Name + "\n"
		sb += s.Instructions + "\n\n"
	}

	return sb
}
