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
