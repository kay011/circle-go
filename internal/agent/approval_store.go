package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ApprovalStatus 表示审批状态。
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
)

// ApprovalStore 抽象审批状态存储，用于多实例场景下共享审批结果。
type ApprovalStore interface {
	CreatePending(ctx context.Context, sessionID, toolCallID, approvalToken string, ttl time.Duration) error
	SetDecision(ctx context.Context, sessionID, toolCallID, approvalToken string, approved bool) error
	GetStatus(ctx context.Context, sessionID, toolCallID string) (ApprovalStatus, bool, error)
	Delete(ctx context.Context, toolCallID string) error
}

type noopApprovalStore struct{}

func (s *noopApprovalStore) CreatePending(ctx context.Context, sessionID, toolCallID, approvalToken string, ttl time.Duration) error {
	return nil
}
func (s *noopApprovalStore) SetDecision(ctx context.Context, sessionID, toolCallID, approvalToken string, approved bool) error {
	return nil
}
func (s *noopApprovalStore) GetStatus(ctx context.Context, sessionID, toolCallID string) (ApprovalStatus, bool, error) {
	return "", false, nil
}
func (s *noopApprovalStore) Delete(ctx context.Context, toolCallID string) error { return nil }

type redisApprovalRecord struct {
	SessionID string         `json:"session_id"`
	ToolCall  string         `json:"tool_call_id"`
	Token     string         `json:"approval_token"`
	Status    ApprovalStatus `json:"status"`
	UpdatedAt int64          `json:"updated_at"`
}

// RedisApprovalStore Redis 审批存储实现。
type RedisApprovalStore struct {
	client *redis.Client
	prefix string
}

// NewRedisApprovalStore 创建 Redis 审批存储。
func NewRedisApprovalStore(client *redis.Client, prefix string) *RedisApprovalStore {
	if prefix == "" {
		prefix = "circle_go"
	}
	return &RedisApprovalStore{
		client: client,
		prefix: prefix,
	}
}

func (s *RedisApprovalStore) CreatePending(ctx context.Context, sessionID, toolCallID, approvalToken string, ttl time.Duration) error {
	if toolCallID == "" || sessionID == "" || approvalToken == "" {
		return errors.New("invalid tool call/session/token")
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	rec := redisApprovalRecord{
		SessionID: sessionID,
		ToolCall:  toolCallID,
		Token:     approvalToken,
		Status:    ApprovalPending,
		UpdatedAt: time.Now().Unix(),
	}
	return s.setRecord(ctx, toolCallID, rec, ttl)
}

func (s *RedisApprovalStore) SetDecision(ctx context.Context, sessionID, toolCallID, approvalToken string, approved bool) error {
	rec, ttl, err := s.getRecord(ctx, toolCallID)
	if err != nil {
		return err
	}
	if rec.SessionID != sessionID {
		return errors.New("session mismatch for tool call")
	}
	if rec.Token == "" || rec.Token != approvalToken {
		return errors.New("invalid approval token")
	}
	if approved {
		rec.Status = ApprovalApproved
	} else {
		rec.Status = ApprovalRejected
	}
	rec.UpdatedAt = time.Now().Unix()
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return s.setRecord(ctx, toolCallID, rec, ttl)
}

func (s *RedisApprovalStore) GetStatus(ctx context.Context, sessionID, toolCallID string) (ApprovalStatus, bool, error) {
	rec, _, err := s.getRecord(ctx, toolCallID)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", false, nil
		}
		return "", false, err
	}
	if rec.SessionID != sessionID {
		return "", false, errors.New("session mismatch for tool call")
	}
	return rec.Status, true, nil
}

func (s *RedisApprovalStore) Delete(ctx context.Context, toolCallID string) error {
	if toolCallID == "" {
		return nil
	}
	return s.client.Del(ctx, s.key(toolCallID)).Err()
}

func (s *RedisApprovalStore) key(toolCallID string) string {
	return fmt.Sprintf("%s:approval:%s", s.prefix, toolCallID)
}

func (s *RedisApprovalStore) setRecord(ctx context.Context, toolCallID string, rec redisApprovalRecord, ttl time.Duration) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.key(toolCallID), b, ttl).Err()
}

func (s *RedisApprovalStore) getRecord(ctx context.Context, toolCallID string) (redisApprovalRecord, time.Duration, error) {
	val, err := s.client.Get(ctx, s.key(toolCallID)).Result()
	if err != nil {
		return redisApprovalRecord{}, 0, err
	}
	var rec redisApprovalRecord
	if err := json.Unmarshal([]byte(val), &rec); err != nil {
		return redisApprovalRecord{}, 0, err
	}
	ttl, err := s.client.TTL(ctx, s.key(toolCallID)).Result()
	if err != nil {
		ttl = 0
	}
	return rec, ttl, nil
}
