package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSkillPromptTextFromDir(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a.md"), []byte("alpha"), 0644); err != nil {
		t.Fatalf("write a.md failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "investment"), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "investment", "SKILL.md"), []byte("beta"), 0644); err != nil {
		t.Fatalf("write SKILL.md failed: %v", err)
	}

	got, err := LoadSkillPromptTextFromDir(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "beta") {
		t.Fatalf("unexpected prompt content: %s", got)
	}
}

func TestLoadToolManifestMapFromSkillsDir_FromSkillMarkdownFrontMatter(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "investment")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	content := `---
name: investment
tools:
  - name: calculator
    id: skill.calc
    intent_tags: ["skill"]
    policy:
      timeout_seconds: 12
      approval: never
---

# Investment Skill
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write SKILL.md failed: %v", err)
	}

	m, err := LoadToolManifestMapFromSkillsDir(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := m["calculator"]
	if !ok {
		t.Fatalf("calculator metadata missing")
	}
	if got.ID != "skill.calc" {
		t.Fatalf("unexpected id: %s", got.ID)
	}
	if got.Policy.TimeoutSeconds != 12 {
		t.Fatalf("unexpected timeout: %d", got.Policy.TimeoutSeconds)
	}
}
