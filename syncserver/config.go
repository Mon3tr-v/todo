package syncserver

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddress   string
	DatabaseURL     string
	JWTSecret       string
	MinIOEndpoint   string
	MinIOAccessKey  string
	MinIOSecretKey  string
	MinIOBucket     string
	MinIOSecure     bool
	AllowSignup     bool
	MaxAttachmentMB int64
}

func ConfigFromEnv() Config {
	maxSize, _ := strconv.ParseInt(env("TODO_MAX_ATTACHMENT_MB", "25"), 10, 64)
	return Config{
		ListenAddress:   env("TODO_LISTEN", ":8080"),
		DatabaseURL:     env("TODO_DATABASE_URL", "postgres://todo:todo@localhost:5432/todo?sslmode=disable"),
		JWTSecret:       os.Getenv("TODO_JWT_SECRET"),
		MinIOEndpoint:   env("TODO_S3_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:  env("TODO_S3_ACCESS_KEY", "todo"),
		MinIOSecretKey:  env("TODO_S3_SECRET_KEY", "todo-development-secret"),
		MinIOBucket:     env("TODO_S3_BUCKET", "todo-attachments"),
		MinIOSecure:     envBool("TODO_S3_SECURE", false),
		AllowSignup:     envBool("TODO_ALLOW_SIGNUPS", false),
		MaxAttachmentMB: maxSize,
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("TODO_DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 32 {
		return errors.New("TODO_JWT_SECRET must contain at least 32 characters")
	}
	if c.MaxAttachmentMB <= 0 || c.MaxAttachmentMB > 1024 {
		return errors.New("TODO_MAX_ATTACHMENT_MB must be between 1 and 1024")
	}
	return nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes"
}
