package longterm

import (
	"math"
	"testing"
	"time"
)

func TestVectorIndexCosineSearch(t *testing.T) {
	ix := NewIndex(t.TempDir())
	sid := "v1"
	a := []float32{1, 0, 0}
	b := []float32{0.9, 0.1, 0}
	_ = ix.Append(sid, Entry{Key: "k1", Value: "alpha", Importance: 1, Timestamp: time.Now(), Embedding: a})
	_ = ix.Append(sid, Entry{Key: "k2", Value: "beta", Importance: 1, Timestamp: time.Now(), Embedding: b})

	hits := ix.Search(sid, []float32{1, 0, 0}, 2)
	if len(hits) < 1 {
		t.Fatalf("hits: %#v", hits)
	}
	if hits[0].Key != "k1" {
		t.Fatalf("expected k1 first, got %s", hits[0].Key)
	}
}

func TestVectorIndexPersistence(t *testing.T) {
	dir := t.TempDir()
	ix := NewIndex(dir)
	sid := "persist"
	v := []float32{0, 1, 0}
	_ = ix.Append(sid, Entry{Key: "x", Value: "y", Importance: 2, Timestamp: time.Now(), Embedding: v})

	ix2 := NewIndex(dir)
	if err := ix2.LoadSession(sid); err != nil {
		t.Fatal(err)
	}
	h := ix2.Search(sid, []float32{0, 1, 0}, 1)
	if len(h) != 1 || math.Abs(float64(h[0].Embedding[1]-1)) > 1e-4 {
		t.Fatalf("reload mismatch %#v", h)
	}
}
