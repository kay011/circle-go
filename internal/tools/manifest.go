package tools

import (
	"fmt"
	"os"
	"path/filepath"
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
// 目录下每个 yaml/yml 文件都可声明 tools 列表，格式与 tools.manifests.yaml 一致。
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
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(e.Name()))
		if !(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			continue
		}
		fileMap, err := LoadToolManifestMap(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		for k, v := range fileMap {
			out[k] = v
		}
	}
	return out, nil
}
