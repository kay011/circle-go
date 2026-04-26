package tools

import (
	"context"
	"testing"
)

func TestDefaultPolicyEngine_FileOperation(t *testing.T) {
	engine := NewDefaultPolicyEngine(nil)

	deny := engine.Evaluate(context.Background(), "file_operation", map[string]interface{}{
		"operation": "read",
		"file_path": "../etc/passwd",
	})
	if deny.Decision != PolicyDeny {
		t.Fatalf("expected deny, got %s", deny.Decision)
	}

	needApproval := engine.Evaluate(context.Background(), "file_operation", map[string]interface{}{
		"operation": "write",
		"file_path": "a.txt",
	})
	if needApproval.Decision != PolicyRequireApproval {
		t.Fatalf("expected require_approval, got %s", needApproval.Decision)
	}

	allow := engine.Evaluate(context.Background(), "file_operation", map[string]interface{}{
		"operation": "read",
		"file_path": "a.txt",
	})
	if allow.Decision != PolicyAllow {
		t.Fatalf("expected allow, got %s", allow.Decision)
	}
}

func TestDefaultPolicyEngine_HTTPClient(t *testing.T) {
	engine := NewDefaultPolicyEngine([]string{"example.com"})

	deny := engine.Evaluate(context.Background(), "http_client", map[string]interface{}{
		"url": "http://127.0.0.1:8080",
	})
	if deny.Decision != PolicyDeny {
		t.Fatalf("expected deny, got %s", deny.Decision)
	}

	needApprovalForMethod := engine.Evaluate(context.Background(), "http_client", map[string]interface{}{
		"method": "POST",
		"url":    "https://api.example.com/v1",
	})
	if needApprovalForMethod.Decision != PolicyRequireApproval {
		t.Fatalf("expected require_approval for non-GET, got %s", needApprovalForMethod.Decision)
	}

	needApprovalForUnknownDomain := engine.Evaluate(context.Background(), "http_client", map[string]interface{}{
		"method": "GET",
		"url":    "https://unknown-domain.com",
	})
	if needApprovalForUnknownDomain.Decision != PolicyRequireApproval {
		t.Fatalf("expected require_approval for unknown domain, got %s", needApprovalForUnknownDomain.Decision)
	}

	allow := engine.Evaluate(context.Background(), "http_client", map[string]interface{}{
		"method": "GET",
		"url":    "https://example.com/docs",
	})
	if allow.Decision != PolicyAllow {
		t.Fatalf("expected allow, got %s", allow.Decision)
	}
}

func TestDefaultPolicyEngine_ManifestApprovalOverride(t *testing.T) {
	engine := NewDefaultPolicyEngine(nil)

	engine.SetToolApprovalPolicy("calculator", "always")
	r1 := engine.Evaluate(context.Background(), "calculator", map[string]interface{}{"expression": "1+1"})
	if r1.Decision != PolicyRequireApproval {
		t.Fatalf("expected always override to require approval, got %s", r1.Decision)
	}

	engine.SetToolApprovalPolicy("http_client", "never")
	r2 := engine.Evaluate(context.Background(), "http_client", map[string]interface{}{
		"method": "POST",
		"url":    "https://api.example.com/v1",
	})
	if r2.Decision != PolicyAllow {
		t.Fatalf("expected never override to allow, got %s", r2.Decision)
	}

	// deny 规则优先级高于 never
	r3 := engine.Evaluate(context.Background(), "http_client", map[string]interface{}{
		"method": "GET",
		"url":    "http://127.0.0.1:8080",
	})
	if r3.Decision != PolicyDeny {
		t.Fatalf("expected deny to remain deny, got %s", r3.Decision)
	}
}
