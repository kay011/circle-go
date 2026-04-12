package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"circle-go/config"
	"circle-go/internal/agent"
	"circle-go/internal/agents"
	"circle-go/internal/llm"
	"circle-go/internal/logging"
	"circle-go/internal/mcp"
	"circle-go/internal/memory"
	"circle-go/internal/observability"
	"circle-go/internal/tools"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// writeSSEData 按 SSE 规范写入文本：拆成多行 data:，由浏览器拼成完整 payload，避免单行 data 内含换行导致前端只收到首行。
func writeSSEData(w http.ResponseWriter, text string) {
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	for _, line := range strings.Split(norm, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprintf(w, "\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// Server API服务器
type Server struct {
	config        *config.Config
	llm           llm.LLM
	toolManager   *tools.ToolManager
	memoryManager *memory.MemoryManager
	logger        *logging.Logger
	agentByID     map[string]*agent.Agent
	defaultAgentID string
	mcpClient      *mcp.MCPClient
	server         *http.Server
	obsShutdown    func(context.Context) error
}

// NewServer 创建API服务器
func NewServer(cfg *config.Config) (*Server, error) {
	obsShutdown, err := observability.Init(context.Background(), observability.Config{
		Enabled:      cfg.Observability.Enabled,
		OTLPEndpoint: cfg.Observability.OTLPEndpoint,
		ServiceName:  cfg.Observability.ServiceName,
		Insecure:     cfg.Observability.Insecure,
	})
	if err != nil {
		return nil, fmt.Errorf("observability: %w", err)
	}

	// 初始化LLM
	llmClient := llm.NewOpenAI(
		cfg.LLM.APIKey,
		cfg.LLM.Model,
		cfg.LLM.BaseURL,
		cfg.LLM.MaxTokens,
		float32(cfg.LLM.Temperature),
	)

	// 初始化工具管理器
	toolManager := tools.NewToolManager()
	toolManager.Register(tools.NewCalculatorTool())
	toolManager.Register(tools.NewWebSearchTool(tools.WebSearchToolConfig{
		SearxInstances: cfg.Search.SearxInstances,
		Mock:           cfg.Search.WebSearchMock,
	}))
	toolManager.Register(tools.NewFileTool())

	mrc := memory.RuntimeConfig{
		ShortTermSize:        cfg.Memory.ShortTermSize,
		LongTermPath:         cfg.Memory.LongTermPath,
		CompressMinMessages:  cfg.Memory.CompressMinMessages,
		CompressTriggerRatio: cfg.Memory.CompressTriggerRatio,
		LongTermMaxItems:     cfg.Memory.LongTermMaxItems,
		ContextInjectionK:    cfg.Memory.ContextInjectionK,
		LongTermMode:         cfg.Memory.LongTermMode,
		EmbeddingModel:       cfg.Memory.EmbeddingModel,
	}
	if cfg.Memory.Redis.Enabled && strings.TrimSpace(cfg.Memory.Redis.Addr) != "" {
		mrc.RedisAddr = strings.TrimSpace(cfg.Memory.Redis.Addr)
		mrc.RedisPassword = cfg.Memory.Redis.Password
		mrc.RedisDB = cfg.Memory.Redis.DB
		mrc.RedisKeyPrefix = cfg.Memory.Redis.KeyPrefix
	}
	memoryManager, err := memory.NewMemoryManagerWithRuntime(mrc)
	if err != nil {
		_ = obsShutdown(context.Background())
		return nil, err
	}
	memoryManager.SetLLM(llmClient)
	memoryManager.SetEmbedder(llmClient)

	specList, defAgentID, err := agents.Load(cfg.Agents.DefinitionsFile)
	if err != nil {
		_ = obsShutdown(context.Background())
		return nil, fmt.Errorf("load agents: %w", err)
	}
	agentByID := make(map[string]*agent.Agent)
	for i := range specList {
		sp := specList[i]
		ag, aerr := agent.NewAgent(llmClient, toolManager, sp)
		if aerr != nil {
			_ = obsShutdown(context.Background())
			return nil, fmt.Errorf("init agent %q: %w", sp.ID, aerr)
		}
		ag.SetMemoryManager(memoryManager)
		agentByID[sp.ID] = ag
	}
	if _, ok := agentByID[defAgentID]; !ok {
		_ = obsShutdown(context.Background())
		return nil, fmt.Errorf("default agent %q missing after load", defAgentID)
	}

	// 初始化MCP客户端
	var mcpClient *mcp.MCPClient
	if cfg.MCP.Enabled {
		mcpClient = mcp.NewMCPClient(cfg.MCP.URL)
	}

	// 初始化日志记录器
	logger := logging.NewLogger(logging.INFO, "API")

	return &Server{
		config:         cfg,
		llm:            llmClient,
		toolManager:    toolManager,
		memoryManager:  memoryManager,
		logger:         logger,
		agentByID:      agentByID,
		defaultAgentID: defAgentID,
		mcpClient:      mcpClient,
		obsShutdown:    obsShutdown,
	}, nil
}

func (s *Server) pickAgent(agentID string) *agent.Agent {
	if agentID != "" {
		if a, ok := s.agentByID[agentID]; ok {
			return a
		}
		s.logger.Warn("unknown agent_id, using default", map[string]interface{}{
			"agent_id":        agentID,
			"default_agent_id": s.defaultAgentID,
		})
	}
	return s.agentByID[s.defaultAgentID]
}

// Start 启动服务器
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/chat/stream", s.handleChatStream)
	mux.HandleFunc("/api/chat/toolcall", s.handleToolCall)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/sessions/{id}", s.handleSession)
	mux.Handle("/", http.FileServer(http.Dir("./frontend")))

	svc := s.config.Observability.ServiceName
	if strings.TrimSpace(svc) == "" {
		svc = "circle-go"
	}
	var handler http.Handler = mux
	if s.config.Observability.Enabled && strings.TrimSpace(s.config.Observability.OTLPEndpoint) != "" {
		handler = otelhttp.NewHandler(mux, svc)
	}

	s.server = &http.Server{
		Addr:           fmt.Sprintf("%s:%s", s.config.Server.Host, s.config.Server.Port),
		Handler:        handler,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   60 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	s.logger.Info("Server starting", map[string]interface{}{
		"host": s.config.Server.Host,
		"port": s.config.Server.Port,
	})
	return s.server.ListenAndServe()
}

// Stop 停止服务器
func (s *Server) Stop() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(ctx); err != nil {
			return err
		}
	}
	if s.obsShutdown != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.obsShutdown(ctx)
	}
	return nil
}

// handleAgents GET /api/agents — 列出已加载的智能体（不含完整 system_prompt）
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	type item struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		ExecutionMode  string `json:"execution_mode"`
		MaxSteps       int    `json:"max_steps"`
		HumanInTheLoop bool   `json:"human_in_the_loop"`
	}
	var list []item
	for _, a := range s.agentByID {
		sp := a.Spec()
		list = append(list, item{
			ID:             sp.ID,
			Name:           sp.DisplayName,
			ExecutionMode:  string(sp.ExecutionMode),
			MaxSteps:       sp.MaxSteps,
			HumanInTheLoop: sp.HumanInTheLoop,
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"default_agent_id": s.defaultAgentID,
		"agents":           list,
	})
}

// handleChat 处理聊天请求
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// 增加请求计数
	logging.IncrMetric("chat_requests_total")
	startTime := time.Now()

	if r.Method != http.MethodPost {
		logging.IncrMetric("chat_requests_errors")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		s.logger.Warn("Method not allowed", map[string]interface{}{
			"method": r.Method,
			"path":   r.URL.Path,
		})
		return
	}

	// 解析请求
	var req struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
		AgentID   string `json:"agent_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.IncrMetric("chat_requests_errors")
		http.Error(w, "Invalid request", http.StatusBadRequest)
		s.logger.Error("Invalid request", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	ag := s.pickAgent(req.AgentID)

	s.logger.Info("Chat request received", map[string]interface{}{
		"session_id":     req.SessionID,
		"message_length": len(req.Message),
		"client_ip":      r.RemoteAddr,
		"agent_id":       ag.Spec().ID,
	})

	// 确保会话存在
	s.memoryManager.AddSession(req.SessionID)

	// 添加用户消息到记忆
	s.memoryManager.AddMessage(req.SessionID, "user", req.Message)

	// 压缩上下文
	if err := s.memoryManager.CompressContext(r.Context(), req.SessionID); err != nil {
		s.logger.Warn("Failed to compress context", map[string]interface{}{
			"error":      err.Error(),
			"session_id": req.SessionID,
		})
	}

	// 提取用户信息并更新用户画像
	if err := s.memoryManager.ExtractUserInfo(r.Context(), req.SessionID); err != nil {
		s.logger.Warn("Failed to extract user info", map[string]interface{}{
			"error":      err.Error(),
			"session_id": req.SessionID,
		})
	}

	// 运行Agent
	response, err := ag.Run(r.Context(), req.SessionID, req.Message)
	if err != nil {
		logging.IncrMetric("chat_requests_errors")
		http.Error(w, fmt.Sprintf("Failed to process message: %v", err), http.StatusInternalServerError)
		s.logger.Error("Failed to process message", map[string]interface{}{
			"error":           err.Error(),
			"session_id":      req.SessionID,
			"processing_time": time.Since(startTime).Milliseconds(),
		})
		return
	}

	// 添加AI响应到记忆
	s.memoryManager.AddMessage(req.SessionID, "assistant", response)

	// 返回响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"response": response,
	})

	processingTime := time.Since(startTime).Milliseconds()
	s.logger.Info("Chat request processed", map[string]interface{}{
		"session_id":      req.SessionID,
		"response_length": len(response),
		"processing_time": processingTime,
	})

	// 记录处理时间
	if processingTime > 5000 {
		logging.IncrMetric("chat_requests_slow")
		s.logger.Warn("Slow chat request", map[string]interface{}{
			"session_id":      req.SessionID,
			"processing_time": processingTime,
		})
	}
}

// handleToolCall 处理工具调用确认请求
func (s *Server) handleToolCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		s.logger.Warn("Method not allowed", map[string]interface{}{
			"method": r.Method,
			"path":   r.URL.Path,
		})
		return
	}

	// 解析请求
	var req struct {
		SessionID  string `json:"session_id"`
		ToolCallID string `json:"tool_call_id"`
		Approved   bool   `json:"approved"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		s.logger.Error("Invalid request", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	s.logger.Info("Tool call confirmation received", map[string]interface{}{
		"session_id":   req.SessionID,
		"tool_call_id": req.ToolCallID,
		"approved":     req.Approved,
	})

	// 这里需要处理工具调用确认，实际实现需要根据 Agent 的设计来完成
	// 目前返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Tool call processed",
	})
}

// handleChatStream 处理流式聊天请求
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	// 增加请求计数
	logging.IncrMetric("chat_stream_requests_total")
	startTime := time.Now()

	if r.Method != http.MethodPost {
		logging.IncrMetric("chat_stream_requests_errors")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		s.logger.Warn("Method not allowed", map[string]interface{}{
			"method": r.Method,
			"path":   r.URL.Path,
		})
		return
	}

	// 解析请求
	var req struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
		AgentID   string `json:"agent_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.IncrMetric("chat_stream_requests_errors")
		http.Error(w, "Invalid request", http.StatusBadRequest)
		s.logger.Error("Invalid request", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	ag := s.pickAgent(req.AgentID)

	s.logger.Info("Stream chat request received", map[string]interface{}{
		"session_id":     req.SessionID,
		"message_length": len(req.Message),
		"client_ip":      r.RemoteAddr,
		"agent_id":       ag.Spec().ID,
	})

	// 确保会话存在
	s.memoryManager.AddSession(req.SessionID)

	// 添加用户消息到记忆
	s.memoryManager.AddMessage(req.SessionID, "user", req.Message)

	// 压缩上下文
	if err := s.memoryManager.CompressContext(r.Context(), req.SessionID); err != nil {
		s.logger.Warn("Failed to compress context", map[string]interface{}{
			"error":      err.Error(),
			"session_id": req.SessionID,
		})
	}

	// 提取用户信息并更新用户画像
	if err := s.memoryManager.ExtractUserInfo(r.Context(), req.SessionID); err != nil {
		s.logger.Warn("Failed to extract user info", map[string]interface{}{
			"error":      err.Error(),
			"session_id": req.SessionID,
		})
	}

	// 设置流式响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// 运行Agent流式
	finalReply, runErr := ag.RunStream(r.Context(), req.SessionID, req.Message, func(chunk string) error {
		writeSSEData(w, chunk)
		return nil
	})

	if runErr != nil {
		logging.IncrMetric("chat_stream_requests_errors")
		writeSSEData(w, fmt.Sprintf("Error: %v", runErr))
		s.logger.Error("Stream chat error", map[string]interface{}{
			"error":           runErr.Error(),
			"session_id":      req.SessionID,
			"processing_time": time.Since(startTime).Milliseconds(),
		})
	} else if finalReply != "" {
		s.memoryManager.AddMessage(req.SessionID, "assistant", finalReply)
	}

	// 发送结束信号
	writeSSEData(w, "[DONE]")

	processingTime := time.Since(startTime).Milliseconds()
	s.logger.Info("Stream chat request processed", map[string]interface{}{
		"session_id":      req.SessionID,
		"processing_time": processingTime,
	})

	// 记录处理时间
	if processingTime > 5000 {
		logging.IncrMetric("chat_stream_requests_slow")
		s.logger.Warn("Slow stream chat request", map[string]interface{}{
			"session_id":      req.SessionID,
			"processing_time": processingTime,
		})
	}
}

// handleSessions 处理会话列表请求
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessions := s.memoryManager.ListSessions()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{
		"sessions": sessions,
	})
}

// handleSession 处理单个会话请求
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := r.URL.Path[len("/api/sessions/"):]

	session := s.memoryManager.GetSession(sessionID)
	if session == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}
