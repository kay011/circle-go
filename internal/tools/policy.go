package tools

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// PolicyDecision 表示策略引擎对工具调用的决策。
type PolicyDecision string

const (
	PolicyAllow           PolicyDecision = "allow"
	PolicyRequireApproval PolicyDecision = "require_approval"
	PolicyDeny            PolicyDecision = "deny"
)

// PolicyResult 表示策略评估结果。
type PolicyResult struct {
	Decision PolicyDecision `json:"decision"`
	Reason   string         `json:"reason,omitempty"`
}

// PolicyEngine 定义工具策略评估接口。
type PolicyEngine interface {
	Evaluate(ctx context.Context, toolName string, args map[string]interface{}) PolicyResult
}

// DefaultPolicyEngine 默认策略引擎（最小可用版本）。
type DefaultPolicyEngine struct {
	TrustedHTTPDomains []string
}

// NewDefaultPolicyEngine 创建默认策略引擎。
func NewDefaultPolicyEngine(trustedHTTPDomains []string) *DefaultPolicyEngine {
	return &DefaultPolicyEngine{
		TrustedHTTPDomains: trustedHTTPDomains,
	}
}

// Evaluate 根据工具和参数返回 allow / require_approval / deny。
func (e *DefaultPolicyEngine) Evaluate(_ context.Context, toolName string, args map[string]interface{}) PolicyResult {
	switch toolName {
	case "file_operation":
		return e.evaluateFileOperation(args)
	case "http_client":
		return e.evaluateHTTPClient(args)
	default:
		return PolicyResult{Decision: PolicyAllow}
	}
}

func (e *DefaultPolicyEngine) evaluateFileOperation(args map[string]interface{}) PolicyResult {
	op := strings.ToLower(strings.TrimSpace(fmt.Sprint(args["operation"])))
	path := strings.TrimSpace(fmt.Sprint(args["file_path"]))

	if strings.Contains(path, "..") {
		return PolicyResult{
			Decision: PolicyDeny,
			Reason:   "file_path 包含非法路径跳转",
		}
	}

	if op == "write" {
		return PolicyResult{
			Decision: PolicyRequireApproval,
			Reason:   "文件写操作需要人工审批",
		}
	}

	return PolicyResult{Decision: PolicyAllow}
}

func (e *DefaultPolicyEngine) evaluateHTTPClient(args map[string]interface{}) PolicyResult {
	rawURL := strings.TrimSpace(fmt.Sprint(args["url"]))
	method := strings.ToUpper(strings.TrimSpace(fmt.Sprint(args["method"])))
	if method == "" || method == "<nil>" {
		method = "GET"
	}
	if rawURL == "" || rawURL == "<nil>" {
		return PolicyResult{
			Decision: PolicyDeny,
			Reason:   "url 不能为空",
		}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return PolicyResult{
			Decision: PolicyDeny,
			Reason:   "url 非法",
		}
	}

	host := strings.ToLower(parsed.Hostname())
	if isLocalOrPrivateHost(host) {
		return PolicyResult{
			Decision: PolicyDeny,
			Reason:   "禁止访问本地或内网地址",
		}
	}

	if method != "GET" {
		return PolicyResult{
			Decision: PolicyRequireApproval,
			Reason:   "非 GET 外部请求需要人工审批",
		}
	}

	// GET 请求：若存在可信域名单，未知域名需要审批；否则默认放行。
	if len(e.TrustedHTTPDomains) > 0 && !isTrustedDomain(host, e.TrustedHTTPDomains) {
		return PolicyResult{
			Decision: PolicyRequireApproval,
			Reason:   "访问非可信域名需要人工审批",
		}
	}

	return PolicyResult{Decision: PolicyAllow}
}

func isTrustedDomain(host string, trusted []string) bool {
	for _, d := range trusted {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func isLocalOrPrivateHost(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	// loopback
	if ip.IsLoopback() {
		return true
	}
	// RFC1918 私网段 + link-local
	privateCIDRs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
	}
	for _, cidr := range privateCIDRs {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
