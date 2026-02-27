package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Role 用户角色
type Role string

const (
	RoleAdmin   Role = "admin"
	RoleManager Role = "manager"
	RoleUser    Role = "user"
)

// StringArray 用于 JSON 字符串数组类型
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return json.Marshal(a)
}

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}

	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, a)
	case string:
		return json.Unmarshal([]byte(v), a)
	default:
		return errors.New("cannot scan type into StringArray")
	}
}

type User struct {
	ID           uuid.UUID   `json:"id"`
	Email        string      `json:"email"`
	PasswordHash string      `json:"-"`
	Name         string      `json:"name"`
	Role         Role        `json:"role"`
	Department   string      `json:"department"`
	QuotaPolicy  string      `json:"quota_policy"`
	Models       StringArray `json:"models"`
	Enabled      bool        `json:"enabled"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	LastLoginAt  *time.Time  `json:"last_login_at,omitempty"`
}

type UserCreateRequest struct {
	Email       string   `json:"email" binding:"required,email"`
	Password    string   `json:"password" binding:"required,min:6"`
	Name        string   `json:"name" binding:"required"`
	Role        Role     `json:"role"`
	Department  string   `json:"department"`
	QuotaPolicy string   `json:"quota_policy"`
	Models      []string `json:"models"`
}

type UserUpdateRequest struct {
	Name        string   `json:"name"`
	Role        Role     `json:"role"`
	Department  string   `json:"department"`
	QuotaPolicy string   `json:"quota_policy"`
	Models      []string `json:"models"`
	Enabled     *bool    `json:"enabled"`
}

type UserResponse struct {
	ID           uuid.UUID   `json:"id"`
	Email        string      `json:"email"`
	Name         string      `json:"name"`
	Role         Role        `json:"role"`
	Department   string      `json:"department"`
	QuotaPolicy  string      `json:"quota_policy"`
	Models       StringArray `json:"models"`
	Enabled      bool        `json:"enabled"`
	CreatedAt    time.Time   `json:"created_at"`
	LastLoginAt  *time.Time  `json:"last_login_at,omitempty"`
}

func (u *User) ToResponse() UserResponse {
	return UserResponse{
		ID:          u.ID,
		Email:       u.Email,
		Name:        u.Name,
		Role:        u.Role,
		Department:  u.Department,
		QuotaPolicy: u.QuotaPolicy,
		Models:      u.Models,
		Enabled:     u.Enabled,
		CreatedAt:   u.CreatedAt,
		LastLoginAt: u.LastLoginAt,
	}
}

// UserStore 用户数据访问层
type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(user *User) error {
	user.ID = uuid.New()
	query := `
		INSERT INTO users (id, email, password_hash, name, role, department, quota_policy, models, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING created_at, updated_at`

	modelsJSON, _ := json.Marshal(user.Models)
	return s.db.QueryRow(query,
		user.ID.String(), user.Email, user.PasswordHash, user.Name, user.Role, user.Department,
		user.QuotaPolicy, string(modelsJSON), user.Enabled,
	).Scan(&user.CreatedAt, &user.UpdatedAt)
}

func (s *UserStore) GetByID(id uuid.UUID) (*User, error) {
	user := &User{}
	query := `
		SELECT id, email, password_hash, name, role, department, quota_policy,
		       models, enabled, created_at, updated_at, last_login_at
		FROM users WHERE id = ?`

	var modelsJSON string
	err := s.db.QueryRow(query, id.String()).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
		&user.Department, &user.QuotaPolicy, &modelsJSON, &user.Enabled,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(modelsJSON), &user.Models)
	return user, nil
}

func (s *UserStore) GetByEmail(email string) (*User, error) {
	user := &User{}
	query := `
		SELECT id, email, password_hash, name, role, department, quota_policy,
		       models, enabled, created_at, updated_at, last_login_at
		FROM users WHERE email = ? AND enabled = 1`

	var modelsJSON string
	err := s.db.QueryRow(query, email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
		&user.Department, &user.QuotaPolicy, &modelsJSON, &user.Enabled,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(modelsJSON), &user.Models)
	return user, nil
}

func (s *UserStore) List(limit, offset int) ([]*User, error) {
	query := `
		SELECT id, email, password_hash, name, role, department, quota_policy,
		       models, enabled, created_at, updated_at, last_login_at
		FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		var modelsJSON string
		err := rows.Scan(
			&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
			&user.Department, &user.QuotaPolicy, &modelsJSON, &user.Enabled,
			&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
		)
		if err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(modelsJSON), &user.Models)
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *UserStore) Update(user *User) error {
	query := `
		UPDATE users SET
			email = ?, name = ?, role = ?, department = ?,
			quota_policy = ?, models = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	modelsJSON, _ := json.Marshal(user.Models)
	_, err := s.db.Exec(query,
		user.Email, user.Name, user.Role, user.Department,
		user.QuotaPolicy, string(modelsJSON), user.Enabled, user.ID.String(),
	)
	return err
}

func (s *UserStore) UpdateLastLogin(id uuid.UUID) error {
	_, err := s.db.Exec("UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?", id.String())
	return err
}

func (s *UserStore) Delete(id uuid.UUID) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id.String())
	return err
}

func (s *UserStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}
