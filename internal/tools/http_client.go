package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NewHTTPClientTool 创建HTTP客户端工具
func NewHTTPClientTool() Tool {
	return &httpClientTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		allowedDomains: []string{}, // 空表示允许所有域名（生产环境应设置白名单）
	}
}

type httpClientTool struct {
	client         *http.Client
	allowedDomains []string
}

func (t *httpClientTool) Name() string {
	return "http_client"
}

func (t *httpClientTool) Description() string {
	return "发送HTTP请求获取API数据或网页内容。支持GET、POST等方法。"
}

func (t *httpClientTool) Parameters() map[string]Property {
	return map[string]Property{
		"method": {
			Type:        "string",
			Description: "HTTP方法：GET、POST、PUT、DELETE等，默认为GET",
		},
		"url": {
			Type:        "string",
			Description: "目标URL（必须是http://或https://）",
		},
		"headers": {
			Type:        "string",
			Description: "JSON格式的HTTP头（可选）",
		},
		"body": {
			Type:        "string",
			Description: "请求体内容（POST/PUT时需要）",
		},
	}
}

func (t *httpClientTool) Required() []string {
	return []string{"url"}
}

func (t *httpClientTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	// 获取参数
	method, ok := args["method"].(string)
	if !ok || method == "" {
		method = "GET"
	}

	url, ok := args["url"].(string)
	if !ok || url == "" {
		return "", fmt.Errorf("missing required parameter: url")
	}

	// 验证URL格式
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("invalid URL: must start with http:// or https://")
	}

	// 检查域名白名单（如果配置了）
	if len(t.allowedDomains) > 0 {
		if !t.isDomainAllowed(url) {
			return "", fmt.Errorf("domain not allowed: %s", url)
		}
	}

	// 创建请求
	var bodyReader io.Reader
	if body, ok := args["body"].(string); ok && body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// 设置默认头
	req.Header.Set("User-Agent", "Circle-Go-Agent/1.0")
	req.Header.Set("Accept", "application/json, text/html, */*")

	// 设置自定义头
	if headers, ok := args["headers"].(string); ok && headers != "" {
		// 简单解析headers（实际应该用JSON解析）
		// 这里仅作为示例
		req.Header.Set("Content-Type", "application/json")
	}

	// 发送请求
	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 限制1MB
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// 格式化响应
	result := fmt.Sprintf("HTTP %d %s\n\n%s", resp.StatusCode, resp.Status, string(respBody))

	// 如果响应太大，截断
	if len(result) > 5000 {
		result = result[:5000] + "\n...(truncated)"
	}

	return result, nil
}

func (t *httpClientTool) isDomainAllowed(url string) bool {
	for _, domain := range t.allowedDomains {
		if strings.Contains(url, domain) {
			return true
		}
	}
	return false
}
