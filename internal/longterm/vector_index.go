package longterm

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Entry 带向量的长期记忆条目（持久化到磁盘）
type Entry struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Importance  int       `json:"importance"`
	Timestamp   time.Time `json:"timestamp"`
	Embedding   []float32 `json:"embedding,omitempty"`
	TextForEmbed string   `json:"-"` // 不参与 json，用于重建嵌入
}

// Index 会话级向量索引（余弦相似度检索 + JSON 落盘）
type Index struct {
	mu        sync.RWMutex
	basePath  string
	bySession map[string][]Entry
}

// NewIndex basePath 下将创建 vector/ 子目录
func NewIndex(basePath string) *Index {
	return &Index{
		basePath:  filepath.Join(basePath, "vector"),
		bySession: make(map[string][]Entry),
	}
}

func (ix *Index) dir() string { return ix.basePath }

// LoadSession 从磁盘加载某会话向量条目
func (ix *Index) LoadSession(sessionID string) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	p := filepath.Join(ix.dir(), sessionID+".json")
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			ix.bySession[sessionID] = nil
			return nil
		}
		return err
	}
	var entries []Entry
	if err := json.Unmarshal(b, &entries); err != nil {
		return err
	}
	ix.bySession[sessionID] = entries
	return nil
}

// SaveSession 持久化某会话
func (ix *Index) SaveSession(sessionID string) error {
	ix.mu.RLock()
	entries := ix.bySession[sessionID]
	ix.mu.RUnlock()
	if err := os.MkdirAll(ix.dir(), 0755); err != nil {
		return err
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ix.dir(), sessionID+".json"), b, 0644)
}

// Append 追加一条（调用方已持有 MemoryManager 锁时，本方法仅操作向量索引锁）
func (ix *Index) Append(sessionID string, e Entry) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.bySession[sessionID] = append(ix.bySession[sessionID], e)
	return ix.saveUnlocked(sessionID)
}

func (ix *Index) saveUnlocked(sessionID string) error {
	if err := os.MkdirAll(ix.dir(), 0755); err != nil {
		return err
	}
	b, err := json.Marshal(ix.bySession[sessionID])
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ix.dir(), sessionID+".json"), b, 0644)
}

// ReplaceAll 用全量条目替换（与内存中长期列表对齐后调用）
func (ix *Index) ReplaceAll(sessionID string, entries []Entry) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	cp := make([]Entry, len(entries))
	copy(cp, entries)
	ix.bySession[sessionID] = cp
	return ix.saveUnlocked(sessionID)
}

// DeleteSession 删除向量文件
func (ix *Index) DeleteSession(sessionID string) error {
	ix.mu.Lock()
	delete(ix.bySession, sessionID)
	ix.mu.Unlock()
	_ = os.Remove(filepath.Join(ix.dir(), sessionID+".json"))
	return nil
}

// Search 余弦相似度 TopK；query 为零向量时按重要性与时间返回前 k 条文本条目
func (ix *Index) Search(sessionID string, query []float32, k int) []Entry {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	entries := ix.bySession[sessionID]
	if len(entries) == 0 || k <= 0 {
		return nil
	}
	if len(query) == 0 || norm(query) == 0 {
		cp := append([]Entry(nil), entries...)
		sort.Slice(cp, func(i, j int) bool {
			if cp[i].Importance != cp[j].Importance {
				return cp[i].Importance > cp[j].Importance
			}
			return cp[i].Timestamp.After(cp[j].Timestamp)
		})
		if len(cp) > k {
			cp = cp[:k]
		}
		return cp
	}
	type scored struct {
		e Entry
		s float64
	}
	var buf []scored
	for _, e := range entries {
		if len(e.Embedding) == 0 || len(e.Embedding) != len(query) {
			continue
		}
		buf = append(buf, scored{e: e, s: cosine(query, e.Embedding)})
	}
	sort.Slice(buf, func(i, j int) bool { return buf[i].s > buf[j].s })
	out := make([]Entry, 0, k)
	for i := 0; i < len(buf) && len(out) < k; i++ {
		out = append(out, buf[i].e)
	}
	if len(out) == 0 {
		return ix.Search(sessionID, nil, k)
	}
	return out
}

func norm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	var na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// SyncFromMemoryItems 用无向量条目覆盖索引中的文本（嵌入清空，由上层重新嵌入）
func (ix *Index) SyncFromMemoryItems(sessionID string, keys []string, values []string, imp []int, ts []time.Time) error {
	if len(keys) != len(values) || len(keys) != len(imp) || len(keys) != len(ts) {
		return fmt.Errorf("sync: slice length mismatch")
	}
	entries := make([]Entry, len(keys))
	for i := range keys {
		entries[i] = Entry{
			Key:          keys[i],
			Value:        values[i],
			Importance:   imp[i],
			Timestamp:    ts[i],
			TextForEmbed: keys[i] + ": " + values[i],
		}
	}
	return ix.ReplaceAll(sessionID, entries)
}
