package api

import (
	"encoding/json"
	"net/http"
)

// ErrorCode 定义 API 统一错误码。
type ErrorCode string

const (
	ErrInvalidMethod    ErrorCode = "invalid_method"
	ErrInvalidRequest   ErrorCode = "invalid_request"
	ErrInternal         ErrorCode = "internal_error"
	ErrAgentRunFailed   ErrorCode = "agent_run_failed"
	ErrToolApproval     ErrorCode = "tool_approval_invalid"
	ErrSessionNotFound  ErrorCode = "session_not_found"
	ErrUnauthorized     ErrorCode = "unauthorized"
	ErrRateLimit        ErrorCode = "rate_limit_exceeded"
	ErrStreamRunFailed  ErrorCode = "stream_run_failed"
	ErrValidationFailed ErrorCode = "validation_failed"
)

// APIErrorPayload 统一错误响应结构。
type APIErrorPayload struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
}

func writeAPIError(w http.ResponseWriter, status int, code ErrorCode, message string, retryable bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIErrorPayload{
		Error: APIError{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	})
}

func writeSSEError(w http.ResponseWriter, code ErrorCode, message string, retryable bool) {
	b, err := json.Marshal(APIErrorPayload{
		Error: APIError{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	})
	if err != nil {
		writeSSEData(w, `{"error":{"code":"internal_error","message":"failed to marshal error","retryable":false}}`)
		return
	}
	writeSSEData(w, string(b))
}
