package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"circle-go/api/middleware"
	"circle-go/config"
	"circle-go/internal/agent"
	"circle-go/internal/auth"
	"circle-go/internal/llm"
	"circle-go/internal/logging"
	"circle-go/internal/mcp"
	"circle-go/internal/memory"
	"circle-go/internal/tools"

	"github.com/redis/go-redis/v9"
)

const appVersion = "1.0.0"

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
	authManager   *auth.AuthManager
	logger        *logging.Logger
	agent         *agent.Agent
	mcpClient     *mcp.MCPClient
	server        *http.Server
	rateLimiter   *middleware.RateLimiter
	startTime     time.Time // 服务器启动时间
}

// NewServer 创建API服务器
func NewServer(cfg *config.Config) *Server {
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
	toolManager.Register(tools.NewWebSearchTool(cfg.Search.SearxInstances))
	toolManager.Register(tools.NewFileTool())
	toolManager.Register(tools.NewHTTPClientTool()) // 新增：HTTP客户端工具
	toolManager.Register(tools.NewInvestmentAnalyzerTool())

	// 初始化记忆管理器
	memoryManager := memory.NewMemoryManager(cfg.Memory.ShortTermSize, cfg.Memory.LongTermPath)

	// 初始化Agent
	humanInTheLoop := true // 启用 human-in-the-loop
	agentInstance := agent.NewAgent(llmClient, toolManager, cfg.AgentRuntime.MaxSteps, humanInTheLoop)
	agentInstance.SetRuntimeLimits(
		cfg.AgentRuntime.MaxSteps,
		cfg.AgentRuntime.MaxToolCalls,
		cfg.AgentRuntime.MaxDuration,
	)
	agentInstance.SetApprovalTimeout(cfg.AgentRuntime.ApprovalTimeout)
	agentInstance.SetPolicyEngine(tools.NewDefaultPolicyEngine(cfg.AgentRuntime.TrustedHTTPDomains))
	agentInstance.SetMemoryManager(memoryManager)
	agentInstance.SetLegacyRoutingPath(cfg.AgentRouting.FeatureFlagLegacy)
	agentInstance.SetToolRetrievalConfig(agent.ToolRetrievalConfig{
		Enabled:       cfg.AgentRouting.Enabled,
		TopK:          cfg.AgentRouting.TopK,
		MinScore:      cfg.AgentRouting.MinScore,
		FallbackToAll: cfg.AgentRouting.FallbackToAll,
	})
	agentInstance.SetToolRouterConfig(agent.ToolRouterConfig{
		Enabled:             cfg.AgentRouting.RouterEnabled,
		MinConfidence:       cfg.AgentRouting.RouterMinConfidence,
		Timeout:             cfg.AgentRouting.RouterTimeout,
		ErrorRerouteEnabled: cfg.AgentRouting.ErrorRerouteEnabled,
		ErrorRerouteTimeout: cfg.AgentRouting.ErrorRerouteTimeout,
	})
	agentInstance.SetResponsePolicyConfig(agent.ResponsePolicyConfig{
		Mode:                 agent.ResponseMode(cfg.AgentRouting.ResponseMode),
		SummarizeTimeout:     cfg.AgentRouting.SummarizeTimeout,
		SummarizeOnToolError: cfg.AgentRouting.SummarizeOnToolError,
	})

	if cfg.Redis.Enabled {
		rdb := redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		agentInstance.SetApprovalStore(agent.NewRedisApprovalStore(rdb, cfg.Redis.Prefix))
	}

	// 设置 LLM 到记忆管理器
	memoryManager.SetLLM(llmClient)

	// 初始化MCP客户端
	var mcpClient *mcp.MCPClient
	if cfg.MCP.Enabled {
		mcpClient = mcp.NewMCPClient(cfg.MCP.URL)
	}

	// 初始化认证管理器
	authManager := auth.NewAuthManager("./auth")

	// 初始化速率限制器（每分钟60个请求）
	rateLimiter := middleware.NewRateLimiter(60, time.Minute)

	// 初始化日志记录器
	logger := logging.NewLogger(logging.INFO, "API")

	// 初始化工具网关（M2：统一工具治理入口）
	toolGateway := tools.NewToolGateway(toolManager, cfg.AgentRuntime.ToolTimeout, func(event tools.AuditEvent) {
		fields := map[string]interface{}{
			"tool_name":   event.ToolName,
			"status":      string(event.Status),
			"duration_ms": event.Duration.Milliseconds(),
		}
		if event.Error != "" {
			fields["error"] = event.Error
		}
		if len(event.Arguments) > 0 {
			fields["arguments"] = event.Arguments
		}
		logger.Info("Tool gateway audit", fields)
	})
	agentInstance.SetToolGateway(toolGateway)

	return &Server{
		config:        cfg,
		llm:           llmClient,
		toolManager:   toolManager,
		memoryManager: memoryManager,
		authManager:   authManager,
		logger:        logger,
		agent:         agentInstance,
		mcpClient:     mcpClient,
		rateLimiter:   rateLimiter,
		startTime:     time.Now(),
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 健康检查端点（不限速）
	http.HandleFunc("/health", s.handleHealth)
	http.HandleFunc("/ready", s.handleReady)
	if s.config.Metrics.Enabled {
		http.HandleFunc(s.config.Metrics.Path, s.handleMetrics)
	}

	// 注册路由（带速率限制）
	http.HandleFunc("/api/auth/register", s.rateLimiter.Middleware(s.handleRegister))
	http.HandleFunc("/api/auth/login", s.rateLimiter.Middleware(s.handleLogin))
	http.HandleFunc("/api/chat", s.rateLimiter.Middleware(s.handleChat))
	http.HandleFunc("/api/chat/stream", s.rateLimiter.Middleware(s.handleChatStream))
	http.HandleFunc("/api/chat/toolcall", s.rateLimiter.Middleware(s.handleToolCall))
	http.HandleFunc("/api/chat/plan", s.rateLimiter.Middleware(s.handleChatWithPlanning)) // 新增：任务规划端点
	http.HandleFunc("/api/sessions", s.rateLimiter.Middleware(s.handleSessions))
	http.HandleFunc("/api/sessions/{id}", s.rateLimiter.Middleware(s.handleSession))

	// 提供静态文件（不限速）
	http.Handle("/", http.FileServer(http.Dir("./frontend")))

	// 创建服务器
	s.server = &http.Server{
		Addr:           fmt.Sprintf("%s:%s", s.config.Server.Host, s.config.Server.Port),
		Handler:        nil, // 使用默认的ServeMux
		ReadTimeout:    s.config.Server.ReadTimeout,
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
		return s.server.Shutdown(ctx)
	}
	return nil
}

// handleChat 处理聊天请求
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// 增加请求计数
	logging.IncrMetricWithLabels("chat_requests_total", map[string]string{"endpoint": "/api/chat"})
	startTime := time.Now()

	if r.Method != http.MethodPost {
		logging.IncrMetricWithLabels("chat_requests_errors", map[string]string{"endpoint": "/api/chat"})
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "Method not allowed", false)
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
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.IncrMetricWithLabels("chat_requests_errors", map[string]string{"endpoint": "/api/chat"})
		writeAPIError(w, http.StatusBadRequest, ErrInvalidRequest, "Invalid request body", false)
		s.logger.Error("Invalid request", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	s.logger.Info("Chat request received", map[string]interface{}{
		"session_id":     req.SessionID,
		"message_length": len(req.Message),
		"client_ip":      r.RemoteAddr,
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
	response, err := s.agent.Run(r.Context(), req.SessionID, req.Message)
	if err != nil {
		logging.IncrMetricWithLabels("chat_requests_errors", map[string]string{"endpoint": "/api/chat"})
		writeAPIError(w, http.StatusInternalServerError, ErrAgentRunFailed, "Failed to process message", true)
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
	logging.ObserveMetricWithLabels("chat_request_duration_seconds", map[string]string{"endpoint": "/api/chat"}, float64(processingTime)/1000.0)
	s.logger.Info("Chat request processed", map[string]interface{}{
		"session_id":      req.SessionID,
		"response_length": len(response),
		"processing_time": processingTime,
	})

	// 记录处理时间
	if processingTime > 5000 {
		logging.IncrMetricWithLabels("chat_requests_slow", map[string]string{"endpoint": "/api/chat"})
		s.logger.Warn("Slow chat request", map[string]interface{}{
			"session_id":      req.SessionID,
			"processing_time": processingTime,
		})
	}
}

// handleChatWithPlanning 处理带任务规划的聊天请求
func (s *Server) handleChatWithPlanning(w http.ResponseWriter, r *http.Request) {
	// 增加请求计数
	logging.IncrMetricWithLabels("chat_planning_requests_total", map[string]string{"endpoint": "/api/chat/plan"})
	startTime := time.Now()

	if r.Method != http.MethodPost {
		logging.IncrMetricWithLabels("chat_planning_requests_errors", map[string]string{"endpoint": "/api/chat/plan"})
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "Method not allowed", false)
		return
	}

	// 解析请求
	var req struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.IncrMetricWithLabels("chat_planning_requests_errors", map[string]string{"endpoint": "/api/chat/plan"})
		writeAPIError(w, http.StatusBadRequest, ErrInvalidRequest, "Invalid request body", false)
		return
	}

	s.logger.Info("Planning chat request received", map[string]interface{}{
		"session_id":     req.SessionID,
		"message_length": len(req.Message),
	})

	// 确保会话存在
	s.memoryManager.AddSession(req.SessionID)

	// 添加用户消息到记忆
	s.memoryManager.AddMessage(req.SessionID, "user", req.Message)

	// 使用任务规划运行Agent
	response, err := s.agent.RunWithPlanning(r.Context(), req.SessionID, req.Message)
	if err != nil {
		logging.IncrMetricWithLabels("chat_planning_requests_errors", map[string]string{"endpoint": "/api/chat/plan"})
		writeAPIError(w, http.StatusInternalServerError, ErrAgentRunFailed, "Failed to process message", true)
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
	logging.ObserveMetricWithLabels("chat_planning_request_duration_seconds", map[string]string{"endpoint": "/api/chat/plan"}, float64(processingTime)/1000.0)
	s.logger.Info("Planning chat processed", map[string]interface{}{
		"session_id":      req.SessionID,
		"response_length": len(response),
		"processing_time": processingTime,
	})
}

// handleToolCall 处理工具调用确认请求
func (s *Server) handleToolCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "Method not allowed", false)
		s.logger.Warn("Method not allowed", map[string]interface{}{
			"method": r.Method,
			"path":   r.URL.Path,
		})
		return
	}

	// 解析请求
	var req struct {
		SessionID     string `json:"session_id"`
		ToolCallID    string `json:"tool_call_id"`
		ApprovalToken string `json:"approval_token"`
		Approved      bool   `json:"approved"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrInvalidRequest, "Invalid request body", false)
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

	if err := s.agent.ResolveToolCallApproval(req.SessionID, req.ToolCallID, req.ApprovalToken, req.Approved); err != nil {
		s.logger.Warn("Tool call confirmation rejected", map[string]interface{}{
			"session_id":   req.SessionID,
			"tool_call_id": req.ToolCallID,
			"error":        err.Error(),
		})
		writeAPIError(w, http.StatusBadRequest, ErrToolApproval, err.Error(), false)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "success",
		"tool_call_id": req.ToolCallID,
		"approved":     req.Approved,
		"message":      "Tool call processed",
	})
}

// handleChatStream 处理流式聊天请求
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	// 增加请求计数
	logging.IncrMetricWithLabels("chat_stream_requests_total", map[string]string{"endpoint": "/api/chat/stream"})
	startTime := time.Now()

	if r.Method != http.MethodPost {
		logging.IncrMetricWithLabels("chat_stream_requests_errors", map[string]string{"endpoint": "/api/chat/stream"})
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "Method not allowed", false)
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
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.IncrMetricWithLabels("chat_stream_requests_errors", map[string]string{"endpoint": "/api/chat/stream"})
		writeAPIError(w, http.StatusBadRequest, ErrInvalidRequest, "Invalid request body", false)
		s.logger.Error("Invalid request", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	s.logger.Info("Stream chat request received", map[string]interface{}{
		"session_id":     req.SessionID,
		"message_length": len(req.Message),
		"client_ip":      r.RemoteAddr,
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
	finalReply, runErr := s.agent.RunStream(r.Context(), req.SessionID, req.Message, func(chunk string) error {
		writeSSEData(w, chunk)
		return nil
	})

	if runErr != nil {
		logging.IncrMetricWithLabels("chat_stream_requests_errors", map[string]string{"endpoint": "/api/chat/stream"})
		writeSSEError(w, ErrStreamRunFailed, runErr.Error(), true)
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
	logging.ObserveMetricWithLabels("chat_stream_request_duration_seconds", map[string]string{"endpoint": "/api/chat/stream"}, float64(processingTime)/1000.0)
	s.logger.Info("Stream chat request processed", map[string]interface{}{
		"session_id":      req.SessionID,
		"processing_time": processingTime,
	})

	// 记录处理时间
	if processingTime > 5000 {
		logging.IncrMetricWithLabels("chat_stream_requests_slow", map[string]string{"endpoint": "/api/chat/stream"})
		s.logger.Warn("Slow stream chat request", map[string]interface{}{
			"session_id":      req.SessionID,
			"processing_time": processingTime,
		})
	}
}

// handleSessions 处理会话列表请求
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "Method not allowed", false)
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
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "Method not allowed", false)
		return
	}
	sessionID := r.URL.Path[len("/api/sessions/"):]

	session := s.memoryManager.GetSession(sessionID)
	if session == nil {
		writeAPIError(w, http.StatusNotFound, ErrSessionNotFound, "Session not found", false)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

// authMiddleware JWT 认证中间件
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeAPIError(w, http.StatusUnauthorized, ErrUnauthorized, "missing authorization header", false)
			return
		}

		// 提取 token
		const prefix = "Bearer "
		if len(authHeader) < len(prefix) || authHeader[:len(prefix)] != prefix {
			writeAPIError(w, http.StatusUnauthorized, ErrUnauthorized, "invalid authorization format", false)
			return
		}

		tokenString := authHeader[len(prefix):]

		// 验证 token
		claims, err := s.authManager.ValidateToken(tokenString)
		if err != nil {
			writeAPIError(w, http.StatusUnauthorized, ErrUnauthorized, err.Error(), false)
			return
		}

		// 将用户信息添加到 context
		ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
		ctx = context.WithValue(ctx, "username", claims.Username)

		next(w, r.WithContext(ctx))
	}
}

// handleRegister 处理用户注册
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "method not allowed", false)
		return
	}

	// 解析请求
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrInvalidRequest, "invalid request body", false)
		return
	}

	// 验证参数
	if req.Username == "" || req.Password == "" || req.Email == "" {
		writeAPIError(w, http.StatusBadRequest, ErrValidationFailed, "missing required fields", false)
		return
	}

	// 验证用户名格式
	if len(req.Username) < 3 || len(req.Username) > 50 {
		writeAPIError(w, http.StatusBadRequest, ErrValidationFailed, "username must be between 3 and 50 characters", false)
		return
	}

	// 验证邮箱格式
	if !isValidEmail(req.Email) {
		writeAPIError(w, http.StatusBadRequest, ErrValidationFailed, "invalid email format", false)
		return
	}

	// 注册用户
	err := s.authManager.Register(req.Username, req.Password, req.Email)
	if err != nil {
		s.logger.Warn("Registration failed", map[string]interface{}{
			"error":    err.Error(),
			"username": req.Username,
		})
		writeAPIError(w, http.StatusBadRequest, ErrValidationFailed, err.Error(), false)
		return
	}

	s.logger.Info("User registered successfully", map[string]interface{}{
		"username": req.Username,
	})

	// 返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Registration successful",
	})
}

// handleLogin 处理用户登录
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "method not allowed", false)
		return
	}

	// 解析请求
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, ErrInvalidRequest, "invalid request body", false)
		return
	}

	// 验证参数
	if req.Username == "" || req.Password == "" {
		writeAPIError(w, http.StatusBadRequest, ErrValidationFailed, "missing required fields", false)
		return
	}

	// 登录用户
	token, err := s.authManager.Login(req.Username, req.Password)
	if err != nil {
		s.logger.Warn("Login failed", map[string]interface{}{
			"error":    err.Error(),
			"username": req.Username,
		})
		writeAPIError(w, http.StatusUnauthorized, ErrUnauthorized, "invalid username or password", false)
		return
	}

	s.logger.Info("User logged in successfully", map[string]interface{}{
		"username": req.Username,
	})

	// 返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":   token,
		"message": "Login successful",
	})
}

// handleHealth 健康检查端点
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "method not allowed", false)
		return
	}

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   appVersion,
		"uptime":    time.Since(s.startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// handleReady 就绪检查端点
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "method not allowed", false)
		return
	}

	// 检查关键组件是否就绪
	checks := make(map[string]bool)

	// 检查 LLM 配置
	checks["llm_configured"] = s.config.LLM.APIKey != ""

	// 检查记忆管理器
	checks["memory_manager"] = s.memoryManager != nil

	// 检查工具管理器
	checks["tool_manager"] = s.toolManager != nil

	// 检查认证管理器
	checks["auth_manager"] = s.authManager != nil

	// 判断整体就绪状态
	allReady := true
	for _, ready := range checks {
		if !ready {
			allReady = false
			break
		}
	}

	status := "ready"
	statusCode := http.StatusOK
	if !allReady {
		status = "not ready"
		statusCode = http.StatusServiceUnavailable
	}

	response := map[string]interface{}{
		"status":    status,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"checks":    checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// handleMetrics 暴露 Prometheus 文本格式指标。
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, ErrInvalidMethod, "method not allowed", false)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	payload := logging.RenderPrometheusMetrics()
	if payload == "" {
		// 保持端点可用，即使当前尚无业务指标。
		payload = "# no_metrics 0\n"
	}
	uptimeSeconds := time.Since(s.startTime).Seconds()
	payload += fmt.Sprintf("# TYPE process_uptime_seconds gauge\nprocess_uptime_seconds %.3f\n", uptimeSeconds)
	payload += fmt.Sprintf("# TYPE circle_go_build_info gauge\ncircle_go_build_info{version=\"%s\"} 1\n", appVersion)
	_, _ = w.Write([]byte(payload))
}

// isValidEmail 简单的邮箱格式验证
func isValidEmail(email string) bool {
	if len(email) < 5 || len(email) > 254 {
		return false
	}

	// 基本格式检查：包含 @ 且前后都有内容
	atIndex := -1
	for i, c := range email {
		if c == '@' {
			atIndex = i
			break
		}
	}

	if atIndex <= 0 || atIndex >= len(email)-1 {
		return false
	}

	localPart := email[:atIndex]
	domainPart := email[atIndex+1:]

	// 本地部分和域名都不能为空
	if len(localPart) == 0 || len(domainPart) == 0 {
		return false
	}

	// 域名必须包含 .
	hasDot := false
	for _, c := range domainPart {
		if c == '.' {
			hasDot = true
			break
		}
	}

	return hasDot
}
