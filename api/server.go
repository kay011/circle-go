package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"circle-go/config"
	"circle-go/internal/agent"
	"circle-go/internal/auth"
	"circle-go/internal/llm"
	"circle-go/internal/logging"
	"circle-go/internal/mcp"
	"circle-go/internal/memory"
	"circle-go/internal/tools"
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
	authManager   *auth.AuthManager
	logger        *logging.Logger
	agent         *agent.Agent
	mcpClient     *mcp.MCPClient
	server        *http.Server

	sessionMu     sync.RWMutex
	sessionTokens map[string]string // token -> username，用于 /me 与登出
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
	toolManager.Register(tools.NewWebSearchTool(tools.WebSearchToolConfig{
		SearxInstances: cfg.Search.SearxInstances,
		Mock:           cfg.Search.WebSearchMock,
	}))
	toolManager.Register(tools.NewFileTool())

	// 初始化记忆管理器
	memoryManager := memory.NewMemoryManager(cfg.Memory.ShortTermSize, cfg.Memory.LongTermPath)

	// 初始化Agent
	humanInTheLoop := true // 启用 human-in-the-loop
	agentInstance := agent.NewAgent(llmClient, toolManager, 5, humanInTheLoop)
	agentInstance.SetMemoryManager(memoryManager)

	// 设置 LLM 到记忆管理器
	memoryManager.SetLLM(llmClient)

	// 初始化MCP客户端
	var mcpClient *mcp.MCPClient
	if cfg.MCP.Enabled {
		mcpClient = mcp.NewMCPClient(cfg.MCP.URL)
	}

	// 初始化认证管理器
	authManager := auth.NewAuthManager("./auth")

	// 初始化日志记录器
	logger := logging.NewLogger(logging.INFO, "API")

	return &Server{
		config:        cfg,
		llm:           llmClient,
		toolManager:   toolManager,
		memoryManager: memoryManager,
		authManager:   authManager,
		logger:        logger,
		agent:         agentInstance,
		mcpClient:     mcpClient,
		sessionTokens: make(map[string]string),
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 注册路由
	http.HandleFunc("/api/auth/register", s.handleRegister)
	http.HandleFunc("/api/auth/login", s.handleLogin)
	http.HandleFunc("/api/auth/me", s.handleAuthMe)
	http.HandleFunc("/api/auth/logout", s.handleLogout)
	http.HandleFunc("/api/chat", s.handleChat)
	http.HandleFunc("/api/chat/stream", s.handleChatStream)
	http.HandleFunc("/api/chat/toolcall", s.handleToolCall)
	http.HandleFunc("/api/sessions", s.handleSessions)
	http.HandleFunc("/api/sessions/{id}", s.handleSession)

	// 提供静态文件
	http.Handle("/", http.FileServer(http.Dir("./frontend")))

	// 创建服务器
	s.server = &http.Server{
		Addr:           fmt.Sprintf("%s:%s", s.config.Server.Host, s.config.Server.Port),
		Handler:        nil, // 使用默认的ServeMux
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
		return s.server.Shutdown(ctx)
	}
	return nil
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
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.IncrMetric("chat_requests_errors")
		http.Error(w, "Invalid request", http.StatusBadRequest)
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
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.IncrMetric("chat_stream_requests_errors")
		http.Error(w, "Invalid request", http.StatusBadRequest)
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

// handleRegister 处理用户注册
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// 验证参数
	if req.Username == "" || req.Password == "" || req.Email == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// 注册用户
	err := s.authManager.Register(req.Username, req.Password, req.Email)
	if err != nil {
		http.Error(w, fmt.Sprintf("Registration failed: %v", err), http.StatusBadRequest)
		return
	}

	// 返回成功响应
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Registration successful",
	})
}

// handleLogin 处理用户登录
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// 验证参数
	if req.Username == "" || req.Password == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// 登录用户
	token, err := s.authManager.Login(req.Username, req.Password)
	if err != nil {
		http.Error(w, fmt.Sprintf("Login failed: %v", err), http.StatusUnauthorized)
		return
	}

	s.sessionMu.Lock()
	s.sessionTokens[token] = req.Username
	s.sessionMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":    token,
		"username": req.Username,
		"message":  "Login successful",
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

// handleAuthMe GET /api/auth/me — 校验 token，返回当前用户名
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tok := bearerToken(r)
	if tok == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	s.sessionMu.RLock()
	u, ok := s.sessionTokens[tok]
	s.sessionMu.RUnlock()
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"username": u})
}

// handleLogout POST /api/auth/logout — 使 token 失效
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tok := bearerToken(r)
	if tok == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	s.sessionMu.Lock()
	delete(s.sessionTokens, tok)
	s.sessionMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
}
