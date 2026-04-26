package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ToolManifest struct {
	Name        string   `yaml:"name"`
	ID          string   `yaml:"id"`
	Version     string   `yaml:"version"`
	IntentTags  []string `yaml:"intent_tags"`
	RiskLevel   string   `yaml:"risk_level"`
	Owner       string   `yaml:"owner"`
	DisplayName string   `yaml:"display_name"`
	Policy      struct {
		TimeoutSeconds int    `yaml:"timeout_seconds"`
		Approval       string `yaml:"approval"`
	} `yaml:"policy"`
}

type ToolManifestFile struct {
	Tools []ToolManifest `yaml:"tools"`
}

func LoadToolManifestMap(path string) (map[string]ToolMetadata, error) {
	out := map[string]ToolMetadata{}
	if path == "" {
		return out, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("read tool manifest failed: %w", err)
	}

	var f ToolManifestFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse tool manifest failed: %w", err)
	}
	for _, t := range f.Tools {
		if t.Name == "" {
			continue
		}
		out[t.Name] = ToolMetadata{
			ID:          t.ID,
			Version:     t.Version,
			IntentTags:  t.IntentTags,
			RiskLevel:   t.RiskLevel,
			Owner:       t.Owner,
			DisplayName: t.DisplayName,
			Policy: ToolMetadataPolicy{
				TimeoutSeconds: t.Policy.TimeoutSeconds,
				Approval:       t.Policy.Approval,
			},
		}
	}
	return out, nil
}

// LoadToolManifestMapFromSkillsDir 从 skills 目录批量加载工具声明。
// 规范：每个 skill 是一个文件夹，优先读取 skills/<skill-id>/SKILL.md(front matter)，回退兼容 skill.yaml（或 skill.yml）。
func LoadToolManifestMapFromSkillsDir(dir string) (map[string]ToolMetadata, error) {
	out := map[string]ToolMetadata{}
	if strings.TrimSpace(dir) == "" {
		return out, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("read skills dir failed: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, e.Name())
		// 1) 优先采用 SKILL.md front matter。
		skillMDPath := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillMDPath); err == nil {
			fileMap, err := loadToolManifestMapFromSkillMarkdown(skillMDPath)
			if err != nil {
				return nil, err
			}
			for k, v := range fileMap {
				out[k] = v
			}
		}

		// 2) 回退兼容 skill.yaml / skill.yml
		manifestPath := filepath.Join(skillDir, "skill.yaml")
		if _, err := os.Stat(manifestPath); err != nil {
			manifestPath = filepath.Join(skillDir, "skill.yml")
			if _, err2 := os.Stat(manifestPath); err2 != nil {
				continue
			}
		}
		fileMap, err := LoadToolManifestMap(manifestPath)
		if err != nil {
			return nil, err
		}
		for k, v := range fileMap {
			// SKILL.md front matter 优先，yaml 只做补充。
			if _, exists := out[k]; !exists {
				out[k] = v
			}
		}
	}
	return out, nil
}

func loadToolManifestMapFromSkillMarkdown(path string) (map[string]ToolMetadata, error) {
	out := map[string]ToolMetadata{}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md failed: %w", err)
	}
	var payload struct {
		Tools []struct {
			Name        string `yaml:"name"`
			ID          string `yaml:"id"`
			Version     string `yaml:"version"`
			RiskLevel   string `yaml:"risk_level"`
			Owner       string `yaml:"owner"`
			DisplayName string `yaml:"display_name"`
			IntentTags  []string `yaml:"intent_tags"`
			Policy      struct {
				TimeoutSeconds int    `yaml:"timeout_seconds"`
				Approval       string `yaml:"approval"`
			} `yaml:"policy"`
		} `yaml:"tools"`
	}

	content := string(data)
	const sep = "---"
	first := strings.Index(content, sep)
	if first != 0 {
		return out, nil
	}
	rest := content[len(sep):]
	second := strings.Index(rest, "\n---")
	if second < 0 {
		return out, nil
	}
	frontMatter := strings.TrimSpace(rest[:second])
	if frontMatter == "" {
		return out, nil
	}
	if err := yaml.Unmarshal([]byte(frontMatter), &payload); err != nil {
		return nil, fmt.Errorf("parse SKILL.md front matter failed: %w", err)
	}
	for _, t := range payload.Tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		out[name] = ToolMetadata{
			ID:          t.ID,
			Version:     t.Version,
			IntentTags:  t.IntentTags,
			RiskLevel:   t.RiskLevel,
			Owner:       t.Owner,
			DisplayName: t.DisplayName,
			Policy: ToolMetadataPolicy{
				TimeoutSeconds: t.Policy.TimeoutSeconds,
				Approval:       t.Policy.Approval,
			},
		}
	}
	return out, nil
}

// LoadSkillPromptTextFromDir 读取 skills 目录中的 prompt 文本并拼接。
// 约定优先读取：
// - skills/<skill>/prompt.md
// - skills/<skill>/SKILL.md
// - skills/*.md
func LoadSkillPromptTextFromDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read skills dir failed: %w", err)
	}

	var snippets []string
	appendIfPrompt := func(path string) {
		b, err := os.ReadFile(path)
		if err != nil {
			return
		}
		text := strings.TrimSpace(string(b))
		if text != "" {
			snippets = append(snippets, text)
		}
	}

	for _, e := range entries {
		name := strings.TrimSpace(e.Name())
		if name == "" {
			continue
		}
		if e.IsDir() {
			appendIfPrompt(filepath.Join(dir, name, "prompt.md"))
			appendIfPrompt(filepath.Join(dir, name, "SKILL.md"))
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".md") {
			appendIfPrompt(filepath.Join(dir, name))
		}
	}
	if len(snippets) == 0 {
		return "", nil
	}
	sort.Strings(snippets)
	return strings.Join(snippets, "\n\n---\n\n"), nil
}
