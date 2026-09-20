package store

import (
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound 记录不存在
var ErrNotFound = errors.New("记录不存在")

// User 管理后台账号（密码只存 PBKDF2 派生值）
type User struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	PassHash           string `json:"-"` // base64
	Salt               string `json:"-"` // base64
	Iterations         int    `json:"-"`
	IsAdmin            bool   `json:"isAdmin"`
	MustChangePassword bool   `json:"mustChangePassword"`
	CreatedAt          string `json:"createdAt"`
	LastLoginAt        string `json:"lastLoginAt"`
	PasswordChangedAt  string `json:"passwordChangedAt"`
}

// Session 服务端会话（存库，重启不掉线）
type Session struct {
	Token     string `json:"-"`
	UserID    int64  `json:"userId"`
	Username  string `json:"username"`
	CreatedAt string `json:"createdAt"`
	ExpiresAt string `json:"expiresAt"`
	UserAgent string `json:"userAgent"`

	IsAdmin            bool   `json:"isAdmin"`
	MustChangePassword bool   `json:"mustChangePassword"`
	LastLoginAt        string `json:"lastLoginAt"`
}

const userCols = `id, username, pass_hash, salt, iterations, is_admin, must_change_password,
	COALESCE(created_at,''), COALESCE(last_login_at,''), COALESCE(password_changed_at,'')`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var isAdmin, mustChange int
	if err := row.Scan(&u.ID, &u.Username, &u.PassHash, &u.Salt, &u.Iterations, &isAdmin, &mustChange,
		&u.CreatedAt, &u.LastLoginAt, &u.PasswordChangedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	u.MustChangePassword = mustChange == 1
	return &u, nil
}

// CountUsers 用户总数（判断是否首次初始化）
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser 新建账号
func (s *Store) CreateUser(username, passHash, salt string, iterations int, isAdmin, mustChange bool) (*User, error) {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO users (username, pass_hash, salt, iterations, is_admin, must_change_password, created_at, password_changed_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		username, passHash, salt, iterations, b2i(isAdmin), b2i(mustChange), now, now)
	if err != nil {
		return nil, err
	}
	return s.GetUserByName(username)
}

// GetUserByName 按用户名查（账号不存在返回 ErrNotFound）
func (s *Store) GetUserByName(username string) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE username = ?`, username))
}

// UpdateUserPassword 改密并清除「必须改密」标记
func (s *Store) UpdateUserPassword(userID int64, passHash, salt string, iterations int) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE users SET pass_hash=?, salt=?, iterations=?, must_change_password=0, password_changed_at=? WHERE id=?`,
		passHash, salt, iterations, now, userID)
	return err
}

// TouchLastLogin 记录最近登录时间
func (s *Store) TouchLastLogin(userID int64) error {
	_, err := s.db.Exec(`UPDATE users SET last_login_at=? WHERE id=?`, time.Now().Format(time.RFC3339), userID)
	return err
}

// CreateSession 建会话
func (s *Store) CreateSession(token string, userID int64, expiresAt time.Time, ua string) error {
	_, err := s.db.Exec(`INSERT INTO sessions (token, user_id, created_at, expires_at, user_agent) VALUES (?,?,?,?,?)`,
		token, userID, time.Now().Format(time.RFC3339), expiresAt.Format(time.RFC3339), ua)
	return err
}

// GetSession 取会话（连带用户信息），过期返回 ErrNotFound
func (s *Store) GetSession(token string) (*Session, error) {
	sess := &Session{}
	var isAdmin, mustChange int
	err := s.db.QueryRow(`SELECT se.token, se.user_id, u.username, se.created_at, se.expires_at, COALESCE(se.user_agent,''),
			u.is_admin, u.must_change_password, COALESCE(u.last_login_at,'')
		FROM sessions se JOIN users u ON u.id = se.user_id
		WHERE se.token = ?`, token).
		Scan(&sess.Token, &sess.UserID, &sess.Username, &sess.CreatedAt, &sess.ExpiresAt, &sess.UserAgent,
			&isAdmin, &mustChange, &sess.LastLoginAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	sess.IsAdmin = isAdmin == 1
	sess.MustChangePassword = mustChange == 1
	exp, perr := time.Parse(time.RFC3339, sess.ExpiresAt)
	if perr == nil && time.Now().After(exp) {
		_ = s.DeleteSession(token)
		return nil, ErrNotFound
	}
	return sess, nil
}

// DeleteSession 删会话（登出）
func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// DeleteOtherSessions 改密后踢掉该用户的其它会话，只保留当前
func (s *Store) DeleteOtherSessions(userID int64, keepToken string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ? AND token <> ?`, userID, keepToken)
	return err
}

// CleanupSessions 清理过期会话
func (s *Store) CleanupSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Format(time.RFC3339))
	return err
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
