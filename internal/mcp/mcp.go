package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// MCP MCP客户端接口
type MCP interface {
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error)
}

// Tool MCP工具结构
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// MCPClient MCP客户端实现
type MCPClient struct {
	url        string
	httpClient *http.Client
}

// NewMCPClient 创建MCP客户端
func NewMCPClient(url string) *MCPClient {
	return &MCPClient{
		url: url,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListTools 列出所有工具
func (c *MCPClient) ListTools(ctx context.Context) ([]Tool, error) {
	// 构建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/tools", c.url), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// 解析响应
	var response struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Tools, nil
}

// CallTool 调用工具
func (c *MCPClient) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	// 构建请求体
	requestBody := struct {
		ToolName string                 `json:"tool_name"`
		Args     map[string]interface{} `json:"args"`
	}{
		ToolName: toolName,
		Args:     args,
	}

	// 序列化请求体
	data, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// 构建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/call", c.url), bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// 解析响应
	var response struct {
		Result string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Result, nil
}


