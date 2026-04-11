package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// User 用户结构
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"` // 密码不返回给客户端
	Email    string `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// AuthManager 认证管理器
type AuthManager struct {
	usersPath string
	users     map[string]User
	jwtSecret string
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
		usersPath: usersPath,
		users:     users,
		jwtSecret: jwtSecret,
	}
}

// Register 注册用户
func (am *AuthManager) Register(username, password, email string) error {
	// 检查用户名是否已存在
	for _, user := range am.users {
		if user.Username == username {
			return fmt.Errorf("username already exists")
		}
		if user.Email == email {
			return fmt.Errorf("email already exists")
		}
	}

	// 简单的密码存储（实际应用中应使用bcrypt等安全的哈希算法）
	// 这里仅作为示例
	// 创建用户
	user := User{
		ID:        generateID(),
		Username:  username,
		Password:  password, // 直接存储密码，实际应用中应哈希
		Email:     email,
		CreatedAt: time.Now(),
	}

	// 添加到用户列表
	am.users[user.ID] = user

	// 保存用户数据
	saveUsers(am.usersPath, am.users)

	return nil
}

// Login 用户登录
func (am *AuthManager) Login(username, password string) (string, error) {
	// 查找用户
	var user User
	found := false
	for _, u := range am.users {
		if u.Username == username {
			user = u
			found = true
			break
		}
	}

	if !found {
		return "", fmt.Errorf("invalid username or password")
	}

	// 简单的密码验证（实际应用中应使用bcrypt等安全的哈希算法）
	// 这里仅作为示例
	if user.Password != password {
		return "", fmt.Errorf("invalid username or password")
	}

	// 生成JWT token
	token, err := am.generateToken(user.ID)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

// ValidateToken 验证JWT token
func (am *AuthManager) ValidateToken(token string) (string, error) {
	// 简单的token验证（实际应用中应使用标准的JWT库）
	// 这里仅作为示例
	if token == "" {
		return "", fmt.Errorf("token is empty")
	}

	// 从token中提取用户ID（实际应用中应解析JWT）
	// 这里仅作为示例
	userID := token // 简化处理

	// 检查用户是否存在
	_, exists := am.users[userID]
	if !exists {
		return "", fmt.Errorf("invalid token")
	}

	return userID, nil
}

// GetUser 获取用户信息
func (am *AuthManager) GetUser(userID string) (User, error) {
	user, exists := am.users[userID]
	if !exists {
		return User{}, fmt.Errorf("user not found")
	}

	// 清除密码
	user.Password = ""

	return user, nil
}

// generateToken 生成JWT token
func (am *AuthManager) generateToken(userID string) (string, error) {
	// 简单的token生成（实际应用中应使用标准的JWT库）
	// 这里仅作为示例
	token := userID + "_" + time.Now().String() + "_" + am.jwtSecret[:8]
	return token, nil
}

// generateJWTSecret 生成JWT密钥
func generateJWTSecret() string {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		// 如果生成失败，使用默认值
		return "default_jwt_secret"
	}
	return base64.StdEncoding.EncodeToString(b)
}

// generateID 生成用户ID
func generateID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// 如果生成失败，使用时间戳
		return fmt.Sprintf("user_%d", time.Now().UnixNano())
	}
	return "user_" + base64.StdEncoding.EncodeToString(b)[:12]
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
func saveUsers(usersPath string, users map[string]User) {
	// 构建用户数据文件路径
	filePath := filepath.Join(usersPath, "users.json")

	// 序列化
	data, err := json.Marshal(users)
	if err != nil {
		fmt.Printf("Failed to marshal users: %v\n", err)
		return
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		fmt.Printf("Failed to write users file: %v\n", err)
	}
}
