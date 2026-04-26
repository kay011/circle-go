package agent

import (
	"sort"
	"strings"

	"circle-go/internal/llm"
)

type ToolRetrievalConfig struct {
	Enabled       bool
	TopK          int
	MinScore      int
	FallbackToAll bool
}

type ToolRetriever struct {
	config ToolRetrievalConfig
	index  []toolSearchDoc
	all    []llm.Tool
}

func NewToolRetriever(all []llm.Tool, config ToolRetrievalConfig) *ToolRetriever {
	cfg := config
	if cfg.TopK <= 0 {
		cfg.TopK = 4
	}
	if cfg.MinScore <= 0 {
		cfg.MinScore = 1
	}
	return &ToolRetriever{
		config: cfg,
		index:  buildToolSearchIndex(all),
		all:    all,
	}
}

func (r *ToolRetriever) Select(query string) []llm.Tool {
	if !r.config.Enabled || len(r.index) == 0 {
		return r.all
	}
	type ranked struct {
		tool  llm.Tool
		score int
	}
	rankedTools := make([]ranked, 0, len(r.index))
	for _, doc := range r.index {
		rankedTools = append(rankedTools, ranked{
			tool:  doc.Tool,
			score: scoreToolForQuery(query, doc),
		})
	}
	sort.SliceStable(rankedTools, func(i, j int) bool {
		return rankedTools[i].score > rankedTools[j].score
	})

	selected := make([]llm.Tool, 0, r.config.TopK)
	for _, item := range rankedTools {
		if item.score < r.config.MinScore {
			continue
		}
		selected = append(selected, item.tool)
		if len(selected) >= r.config.TopK {
			break
		}
	}
	if len(selected) == 0 && r.config.FallbackToAll {
		return r.all
	}
	if len(selected) == 0 {
		return r.all
	}
	return selected
}

func buildToolSearchIndex(llmTools []llm.Tool) []toolSearchDoc {
	docs := make([]toolSearchDoc, 0, len(llmTools))
	for _, t := range llmTools {
		keywords := []string{strings.ToLower(strings.TrimSpace(t.Name))}
		keywords = append(keywords, strings.ToLower(strings.TrimSpace(t.Description)))
		keywords = append(keywords, strings.Fields(strings.ToLower(strings.TrimSpace(t.Name)))...)
		keywords = append(keywords, strings.Fields(strings.ToLower(strings.TrimSpace(t.Description)))...)
		for pName, p := range t.Parameters.Properties {
			keywords = append(keywords,
				strings.ToLower(strings.TrimSpace(pName)),
				strings.ToLower(strings.TrimSpace(p.Description)),
			)
			keywords = append(keywords, strings.Fields(strings.ToLower(strings.TrimSpace(pName)))...)
			keywords = append(keywords, strings.Fields(strings.ToLower(strings.TrimSpace(p.Description)))...)
		}
		docs = append(docs, toolSearchDoc{
			Tool:     t,
			Keywords: dedupeNonEmpty(keywords),
		})
	}
	return docs
}

func dedupeNonEmpty(items []string) []string {
	set := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, v := range items {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := set[v]; ok {
			continue
		}
		set[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func scoreToolForQuery(query string, doc toolSearchDoc) int {
	score := 0
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return score
	}
	for _, kw := range doc.Keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(q, kw) {
			score += 4
			continue
		}
		if len(kw) >= 2 && strings.Contains(kw, q) {
			score += 1
		}
	}
	return score
}
