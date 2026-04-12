package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// User 用户结构
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // 密码哈希，不返回给客户端
	Email        string    `json:"email"`
	CreatedAt    time.Time `json:"created_at"`
	LastLoginAt  time.Time `json:"last_login_at,omitempty"`
}

// Claims JWT Claims 结构
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// AuthManager 认证管理器
type AuthManager struct {
	usersPath   string
	users       map[string]User
	jwtSecret   []byte
	tokenExpiry time.Duration
	mu          sync.RWMutex
}

// NewAuthManager 创建认证管理器
func NewAuthManager(usersPath string) *AuthManager {
	// 确保用户数据目录存在
	if err := os.MkdirAll(usersPath, 0755); err != nil {
		fmt.Printf("Failed to create users directory: %v\n", err)
	}

	// 生成JWT密钥
	jwtSecret := generateJWTSecret()

	// 加载用户数据
	users := loadUsers(usersPath)

	return &AuthManager{
		usersPath:   usersPath,
		users:       users,
		jwtSecret:   jwtSecret,
		tokenExpiry: 24 * time.Hour, // Token 有效期24小时
		mu:          sync.RWMutex{},
	}
}

// Register 注册用户
func (am *AuthManager) Register(username, password, email string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// 检查用户名是否已存在
	for _, user := range am.users {
		if user.Username == username {
			return fmt.Errorf("username already exists")
		}
		if user.Email == email {
			return fmt.Errorf("email already exists")
		}
	}

	// 验证密码强度
	if err := validatePassword(password); err != nil {
		return err
	}

	// 使用 bcrypt 哈希密码
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// 创建用户
	user := User{
		ID:           generateID(),
		Username:     username,
		PasswordHash: string(passwordHash),
		Email:        email,
		CreatedAt:    time.Now(),
	}

	// 添加到用户列表
	am.users[user.ID] = user

	// 保存用户数据
	if err := saveUsers(am.usersPath, am.users); err != nil {
		// 回滚：删除刚添加的用户
		delete(am.users, user.ID)
		return fmt.Errorf("failed to save users: %w", err)
	}

	return nil
}

// Login 用户登录
func (am *AuthManager) Login(username, password string) (string, error) {
	am.mu.RLock()
	// 查找用户
	var user User
	var userID string
	found := false
	for id, u := range am.users {
		if u.Username == username {
			user = u
			userID = id
			found = true
			break
		}
	}
	am.mu.RUnlock()

	if !found {
		// 使用相同的时间进行bcrypt比较，防止时序攻击
		bcrypt.CompareHashAndPassword([]byte("$2a$10$invalid.invalid.invalid"), []byte(password))
		return "", fmt.Errorf("invalid username or password")
	}

	// 使用 bcrypt 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", fmt.Errorf("invalid username or password")
	}

	// 生成JWT token
	token, err := am.generateToken(userID, username)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	// 更新最后登录时间
	am.mu.Lock()
	if u, exists := am.users[userID]; exists {
		u.LastLoginAt = time.Now()
		am.users[userID] = u
		saveUsers(am.usersPath, am.users)
	}
	am.mu.Unlock()

	return token, nil
}

// ValidateToken 验证JWT token
func (am *AuthManager) ValidateToken(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("token is empty")
	}

	// 解析 JWT token
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// 验证签名算法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return am.jwtSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}

	// 检查用户是否存在
	am.mu.RLock()
	_, exists := am.users[claims.UserID]
	am.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	return claims, nil
}

// GetUser 获取用户信息
func (am *AuthManager) GetUser(userID string) (User, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	user, exists := am.users[userID]
	if !exists {
		return User{}, fmt.Errorf("user not found")
	}

	// 清除敏感信息
	user.PasswordHash = ""

	return user, nil
}

// generateToken 生成JWT token
func (am *AuthManager) generateToken(userID, username string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(am.tokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "circle-go",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(am.jwtSecret)
}

// validatePassword 验证密码强度
func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false

	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasDigit = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit {
		return fmt.Errorf("password must contain at least one uppercase letter, one lowercase letter, and one digit")
	}

	return nil
}

// generateJWTSecret 生成JWT密钥
func generateJWTSecret() []byte {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		// 如果生成失败，使用时间戳作为备选（生产环境应使用环境变量）
		return []byte(fmt.Sprintf("default_secret_%d", time.Now().UnixNano()))
	}
	return b
}

// generateID 生成用户ID
func generateID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// 如果生成失败，使用时间戳
		return fmt.Sprintf("user_%d", time.Now().UnixNano())
	}
	return "user_" + base64.URLEncoding.EncodeToString(b)[:12]
}

// loadUsers 加载用户数据
func loadUsers(usersPath string) map[string]User {
	users := make(map[string]User)

	// 构建用户数据文件路径
	filePath := filepath.Join(usersPath, "users.json")

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("Failed to read users file: %v\n", err)
		}
		return users
	}

	// 反序列化
	if err := json.Unmarshal(data, &users); err != nil {
		fmt.Printf("Failed to unmarshal users: %v\n", err)
		return users
	}

	return users
}

// saveUsers 保存用户数据
func saveUsers(usersPath string, users map[string]User) error {
	// 构建用户数据文件路径
	filePath := filepath.Join(usersPath, "users.json")

	// 序列化
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal users: %w", err)
	}

	// 写入文件（使用原子写入，先写临时文件再重命名）
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temporary users file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("failed to rename users file: %w", err)
	}

	return nil
}
