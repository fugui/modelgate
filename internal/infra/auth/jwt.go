package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"modelgate/internal/repository"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

// Claims JWT 声明
type Claims struct {
	UserID uuid.UUID   `json:"user_id"`
	Email  string      `json:"email"`
	Name   string      `json:"name"`
	Role   entity.Role `json:"role"`
	jwt.RegisteredClaims
}

// JWTManager JWT 管理器
type JWTManager struct {
	secret      []byte
	expireHours int
}

func NewJWTManager(secret string, expireHours int) *JWTManager {
	return &JWTManager{
		secret:      []byte(secret),
		expireHours: expireHours,
	}
}

// Generate 生成 JWT Token
func (m *JWTManager) Generate(user *entity.User) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		Name:   user.Name,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(m.expireHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "modelgate",
			Subject:   user.ID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Validate 验证 JWT Token
func (m *JWTManager) Validate(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}
