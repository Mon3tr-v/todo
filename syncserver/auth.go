package syncserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/argon2"
)

type authUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Admin    bool   `json:"admin"`
}

type tokenClaims struct {
	Username string `json:"username"`
	Admin    bool   `json:"admin"`
	jwt.RegisteredClaims
}

func hashPassword(password string) (string, error) {
	if len(password) < 10 || len(password) > 256 {
		return "", errors.New("password must contain between 10 and 256 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	memory, iterations, parallelism := uint32(64*1024), uint32(3), uint8(2)
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func (s *Server) CreateUser(ctx context.Context, username, password string, admin bool) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if len(username) < 3 || len(username) > 32 {
		return "", errors.New("username must contain between 3 and 32 characters")
	}
	for _, char := range username {
		if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '_' && char != '-' && char != '.' {
			return "", errors.New("username contains unsupported characters")
		}
	}
	hash, err := hashPassword(password)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	_, err = s.db.Exec(ctx, `INSERT INTO users(id, username, password_hash, is_admin) VALUES ($1, $2, $3, $4)`, id, username, hash, admin)
	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (s *Server) authenticate(ctx context.Context, username, password string) (authUser, error) {
	var user authUser
	var hash string
	var disabled bool
	err := s.db.QueryRow(ctx, `SELECT id, username, password_hash, is_admin, disabled FROM users WHERE username = $1`, strings.ToLower(strings.TrimSpace(username))).
		Scan(&user.ID, &user.Username, &hash, &user.Admin, &disabled)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (disabled || !verifyPassword(hash, password)) {
		return authUser{}, errors.New("invalid username or password")
	}
	return user, err
}

func (s *Server) issueTokens(ctx context.Context, user authUser) (string, string, error) {
	now := time.Now().UTC()
	claims := tokenClaims{Username: user.Username, Admin: user.Admin, RegisteredClaims: jwt.RegisteredClaims{
		Subject: user.ID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		Issuer: "todo-sync", ID: uuid.NewString(),
	}}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.config.JWTSecret))
	if err != nil {
		return "", "", err
	}
	refreshBytes := make([]byte, 48)
	if _, err := rand.Read(refreshBytes); err != nil {
		return "", "", err
	}
	refresh := base64.RawURLEncoding.EncodeToString(refreshBytes)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(refresh)))
	_, err = s.db.Exec(ctx, `INSERT INTO refresh_tokens(id, user_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`, uuid.NewString(), user.ID, hash, now.Add(30*24*time.Hour))
	return access, refresh, err
}

func (s *Server) parseAccessToken(value string) (authUser, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "Bearer ")
	claims := &tokenClaims{}
	token, err := jwt.ParseWithClaims(value, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.config.JWTSecret), nil
	}, jwt.WithIssuer("todo-sync"), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || claims.Subject == "" {
		return authUser{}, errors.New("invalid access token")
	}
	return authUser{ID: claims.Subject, Username: claims.Username, Admin: claims.Admin}, nil
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
