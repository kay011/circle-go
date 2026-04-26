package tools

import (
	"os"
	"path/filepath"
	"testing"

	"circle-go/config"
)

func TestBuildToolManagerFromConfig(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			Enabled: []string{"calculator", "fund_compare"},
		},
	}

	tm, err := BuildToolManagerFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tm.Get("calculator") == nil {
		t.Fatalf("calculator should be registered")
	}
	if tm.Get("fund_compare") == nil {
		t.Fatalf("fund_compare should be registered")
	}
	if tm.Get("web_search") != nil {
		t.Fatalf("web_search should not be registered")
	}
}

func TestBuildToolManagerFromConfig_UnknownTool(t *testing.T) {
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			Enabled: []string{"not_exists"},
		},
	}
	_, err := BuildToolManagerFromConfig(cfg)
	if err == nil {
		t.Fatalf("expected error for unknown tool")
	}
}

func TestBuildToolManagerFromConfig_WithManifestMetadata(t *testing.T) {
	tmp := t.TempDir()
	manifestPath := filepath.Join(tmp, "tools.manifests.yaml")
	content := []byte("tools:\n  - name: \"calculator\"\n    id: \"base.calc\"\n    version: \"1.0.0\"\n    intent_tags: [\"计算\",\"数学\"]\n    policy:\n      timeout_seconds: 9\n      approval: \"allow\"\n")
	if err := os.WriteFile(manifestPath, content, 0644); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}

	cfg := &config.Config{
		Tools: config.ToolsConfig{
			ManifestPath: manifestPath,
			Enabled:      []string{"calculator"},
		},
	}
	tm, err := BuildToolManagerFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tool := tm.Get("calculator")
	if tool == nil {
		t.Fatalf("calculator should be registered")
	}
	provider, ok := tool.(ToolMetadataProvider)
	if !ok {
		t.Fatalf("calculator should provide metadata from manifest")
	}
	md := provider.Metadata()
	if md.ID != "base.calc" {
		t.Fatalf("unexpected metadata id: %s", md.ID)
	}
	if md.Policy.TimeoutSeconds != 9 {
		t.Fatalf("unexpected timeout policy: %d", md.Policy.TimeoutSeconds)
	}
}

func TestBuildToolPolicyMapFromConfig(t *testing.T) {
	tmp := t.TempDir()
	manifestPath := filepath.Join(tmp, "tools.manifests.yaml")
	content := []byte("tools:\n  - name: \"calculator\"\n    policy:\n      timeout_seconds: 11\n")
	if err := os.WriteFile(manifestPath, content, 0644); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			ManifestPath: manifestPath,
			Enabled:      []string{"calculator"},
		},
	}
	policies, err := BuildToolPolicyMapFromConfig(cfg, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p, ok := policies["calculator"]
	if !ok {
		t.Fatalf("calculator policy missing")
	}
	if p.Timeout.Seconds() != 11 {
		t.Fatalf("unexpected timeout: %v", p.Timeout)
	}
}

func TestBuildToolApprovalPolicyMapFromConfig(t *testing.T) {
	tmp := t.TempDir()
	manifestPath := filepath.Join(tmp, "tools.manifests.yaml")
	content := []byte("tools:\n  - name: \"calculator\"\n    policy:\n      approval: \"always\"\n")
	if err := os.WriteFile(manifestPath, content, 0644); err != nil {
		t.Fatalf("write manifest failed: %v", err)
	}
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			ManifestPath: manifestPath,
			Enabled:      []string{"calculator"},
		},
	}
	m, err := BuildToolApprovalPolicyMapFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["calculator"] != "always" {
		t.Fatalf("unexpected approval mode: %q", m["calculator"])
	}
}

func TestBuildToolManagerFromConfig_SkillsOverride(t *testing.T) {
	tmp := t.TempDir()
	manifestPath := filepath.Join(tmp, "tools.manifests.yaml")
	skillsDir := filepath.Join(tmp, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills failed: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("tools:\n  - name: \"calculator\"\n    id: \"base.calc\"\n    intent_tags: [\"base\"]\n"), 0644); err != nil {
		t.Fatalf("write base manifest failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "skill.yaml"), []byte("tools:\n  - name: \"calculator\"\n    id: \"skill.calc\"\n    intent_tags: [\"skill\"]\n"), 0644); err != nil {
		t.Fatalf("write skill manifest failed: %v", err)
	}
	cfg := &config.Config{
		Tools: config.ToolsConfig{
			ManifestPath: manifestPath,
			Enabled:      []string{"calculator"},
		},
		Skills: config.SkillsConfig{
			Enabled: true,
			Path:    skillsDir,
		},
	}
	tm, err := BuildToolManagerFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tool := tm.Get("calculator")
	provider, ok := tool.(ToolMetadataProvider)
	if !ok {
		t.Fatalf("calculator should provide metadata")
	}
	if provider.Metadata().ID != "skill.calc" {
		t.Fatalf("expected skills override id, got %s", provider.Metadata().ID)
	}
}
