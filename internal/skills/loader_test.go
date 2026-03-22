package skills

import (
	"strings"
	"testing"
)

const testSkillsDir = "../../skills"

func TestLoad(t *testing.T) {
	loader := Load(testSkillsDir)
	all := loader.All()
	if len(all) == 0 {
		t.Fatal("未加载到任何 skill，请确认 skills/ 目录存在且包含 SKILL.md")
	}

	found := false
	for _, s := range all {
		if s.Name == "query_standard" {
			found = true
			if s.Phase != "intent" {
				t.Errorf("query_standard phase 期望 'intent'，实际 '%s'", s.Phase)
			}
			if s.ReferencesDir == "" {
				t.Error("query_standard 缺少 references_dir")
			}
			if s.Instructions == "" {
				t.Error("query_standard 指令（正文）为空")
			}
			t.Logf("✓ skill 加载成功: name=%s phase=%s references_dir=%s instructions_len=%d",
				s.Name, s.Phase, s.ReferencesDir, len(s.Instructions))
		}
	}
	if !found {
		t.Error("未找到 query_standard skill")
	}
}

func TestByPhase(t *testing.T) {
	loader := Load(testSkillsDir)
	intentSkills := loader.ByPhase("intent")
	if len(intentSkills) == 0 {
		t.Fatal("phase=intent 的 skill 为空")
	}
	t.Logf("✓ phase=intent skills: %d 个", len(intentSkills))
}

func TestBuildPromptSection(t *testing.T) {
	loader := Load(testSkillsDir)
	section := loader.BuildPromptSection("intent")
	if section == "" {
		t.Fatal("BuildPromptSection 返回空字符串")
	}
	if !strings.Contains(section, "query_standard") {
		t.Error("Prompt section 中未包含 query_standard")
	}
	t.Logf("✓ Prompt section 长度: %d bytes", len(section))
	t.Logf("预览:\n%s", section[:min(300, len(section))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
