package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_AllowThenBlock(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	if !rl.Allow("192.168.1.1") || !rl.Allow("192.168.1.1") {
		t.Fatal("expected first two allows")
	}
	if rl.Allow("192.168.1.1") {
		t.Fatal("expected third request blocked")
	}
}

func TestRateLimiter_Middleware_429(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	h := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first: %d", rr.Code)
	}

	rr2 := httptest.NewRecorder()
	h(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second want 429, got %d", rr2.Code)
	}
}
