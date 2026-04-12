package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"circle-go/internal/utils"

	"github.com/sashabaranov/go-openai"
)

// LLM 大语言模型接口
type LLM interface {
	Chat(ctx context.Context, messages []Message) (string, error)
	ChatStream(ctx context.Context, messages []Message, callback func(chunk string) error) error
	FunctionCall(ctx context.Context, messages []Message, tools []Tool) (string, *FunctionCall, error)
}

// Message 消息结构
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Tool 工具定义
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
}

// ToolParameters 工具参数
type ToolParameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property 属性定义
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// FunctionCall 函数调用结构
type FunctionCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// OpenAI LLM 实现（带重试和熔断）
type OpenAI struct {
	client      *openai.Client
	model       string
	maxTokens   int
	temperature float32
	retryConfig utils.RetryConfig
	breaker     *utils.CircuitBreaker
}

// NewOpenAI 创建OpenAI LLM实例
func NewOpenAI(apiKey, model, baseURL string, maxTokens int, temperature float32) *OpenAI {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	// 创建断路器
	breaker := utils.NewCircuitBreaker(utils.DefaultCircuitBreakerConfig)
	breaker.SetStateChangeCallback(func(oldState, newState utils.CircuitState) {
		fmt.Printf("[LLM Circuit Breaker] State changed: %s -> %s\n", oldState, newState)
	})

	return &OpenAI{
		client:      openai.NewClientWithConfig(config),
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		retryConfig: utils.DefaultRetryConfig,
		breaker:     breaker,
	}
}

// SetRetryConfig 设置重试配置
func (o *OpenAI) SetRetryConfig(config utils.RetryConfig) {
	o.retryConfig = config
}

// GetBreakerMetrics 获取断路器指标
func (o *OpenAI) GetBreakerMetrics() utils.CircuitBreakerMetrics {
	return o.breaker.GetMetrics()
}

// Chat 发送聊天请求（带重试和熔断）
func (o *OpenAI) Chat(ctx context.Context, messages []Message) (string, error) {
	// 转换消息格式
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	var result string
	err := o.breaker.Execute(func() error {
		return utils.RetryWithBackoff(ctx, o.retryConfig, nil, func() error {
			// 发送请求
			resp, err := o.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
				Model:       o.model,
				Messages:    openaiMessages,
				MaxTokens:   o.maxTokens,
				Temperature: o.temperature,
			})
			if err != nil {
				return fmt.Errorf("failed to create chat completion: %w", err)
			}

			if len(resp.Choices) == 0 {
				return fmt.Errorf("no choices returned")
			}

			result = resp.Choices[0].Message.Content
			return nil
		})
	})

	return result, err
}

// ChatStream 流式发送聊天请求
func (o *OpenAI) ChatStream(ctx context.Context, messages []Message, callback func(chunk string) error) error {
	// 转换消息格式
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 发送流式请求
	stream, err := o.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:       o.model,
		Messages:    openaiMessages,
		MaxTokens:   o.maxTokens,
		Temperature: o.temperature,
		Stream:      true,
	})
	if err != nil {
		return fmt.Errorf("failed to create chat completion stream: %w", err)
	}
	defer stream.Close()

	// 处理流式响应
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}

		if len(response.Choices) > 0 && response.Choices[0].Delta.Content != "" {
			if err := callback(response.Choices[0].Delta.Content); err != nil {
				return err
			}
		}
	}

	return nil
}

// FunctionCall 发送函数调用请求（带重试和熔断）
func (o *OpenAI) FunctionCall(ctx context.Context, messages []Message, tools []Tool) (string, *FunctionCall, error) {
	// 转换消息格式
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 转换工具格式
	openaiTools := make([]openai.Tool, len(tools))
	for i, tool := range tools {
		// 转换参数格式
		paramsMap := map[string]interface{}{
			"type":       tool.Parameters.Type,
			"properties": tool.Parameters.Properties,
		}
		if len(tool.Parameters.Required) > 0 {
			paramsMap["required"] = tool.Parameters.Required
		}

		openaiTools[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  paramsMap,
			},
		}
	}

	var content string
	var functionCall *FunctionCall

	err := o.breaker.Execute(func() error {
		return utils.RetryWithBackoff(ctx, o.retryConfig, nil, func() error {
			// 发送请求
			resp, err := o.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
				Model:       o.model,
				Messages:    openaiMessages,
				Tools:       openaiTools,
				MaxTokens:   o.maxTokens,
				Temperature: o.temperature,
			})
			if err != nil {
				return fmt.Errorf("failed to create chat completion: %w", err)
			}

			if len(resp.Choices) == 0 {
				return fmt.Errorf("no choices returned")
			}

			choice := resp.Choices[0]
			if choice.Message.ToolCalls != nil && len(choice.Message.ToolCalls) > 0 {
				// 函数调用
				toolCall := choice.Message.ToolCalls[0]
				arguments := make(map[string]interface{})
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &arguments); err != nil {
					return fmt.Errorf("failed to parse arguments: %w", err)
				}

				functionCall = &FunctionCall{
					Name:      toolCall.Function.Name,
					Arguments: arguments,
				}
				return nil
			}

			// 普通响应
			content = choice.Message.Content
			return nil
		})
	})

	return content, functionCall, err
}
