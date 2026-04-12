package agents

import (
	"fmt"
	"os"
	"strings"

	"circle-go/internal/agent"

	"gopkg.in/yaml.v3"
)

type definitionsFile struct {
	DefaultAgent string       `yaml:"default_agent"`
	Agents       []agent.Spec `yaml:"agents"`
}

// Load 从 YAML 加载智能体定义。path 为空或文件不存在时返回内置默认智能体。
func Load(path string) ([]agent.Spec, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		d := agent.DefaultSpec()
		return []agent.Spec{d}, d.ID, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			d := agent.DefaultSpec()
			return []agent.Spec{d}, d.ID, nil
		}
		return nil, "", fmt.Errorf("agents file stat %q: %w", path, err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read agents file %q: %w", path, err)
	}

	var f definitionsFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, "", fmt.Errorf("parse agents yaml: %w", err)
	}
	if len(f.Agents) == 0 {
		d := agent.DefaultSpec()
		return []agent.Spec{d}, d.ID, nil
	}

	seen := make(map[string]struct{})
	out := make([]agent.Spec, 0, len(f.Agents))
	for i := range f.Agents {
		sp := f.Agents[i]
		if err := sp.Normalize(); err != nil {
			return nil, "", fmt.Errorf("agent[%d] id=%q: %w", i, sp.ID, err)
		}
		if _, dup := seen[sp.ID]; dup {
			return nil, "", fmt.Errorf("duplicate agent id %q", sp.ID)
		}
		seen[sp.ID] = struct{}{}
		out = append(out, sp)
	}

	defID := strings.TrimSpace(f.DefaultAgent)
	if defID == "" {
		defID = out[0].ID
	}
	found := false
	for _, sp := range out {
		if sp.ID == defID {
			found = true
			break
		}
	}
	if !found {
		return nil, "", fmt.Errorf("default_agent %q not found in agents list", defID)
	}

	return out, defID, nil
}
