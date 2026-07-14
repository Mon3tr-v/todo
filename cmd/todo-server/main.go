package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"TODO/syncserver"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	config := syncserver.ConfigFromEnv()
	ctx := context.Background()
	server, err := syncserver.New(ctx, config, logger)
	if err != nil {
		logger.Error("server initialisation failed", "error", err)
		os.Exit(1)
	}
	defer server.Close()
	if len(os.Args) >= 3 && os.Args[1] == "admin" && os.Args[2] == "create-user" {
		username := strings.TrimSpace(os.Getenv("TODO_ADMIN_USERNAME"))
		password := os.Getenv("TODO_ADMIN_PASSWORD")
		if username == "" || password == "" {
			logger.Error("TODO_ADMIN_USERNAME and TODO_ADMIN_PASSWORD are required")
			os.Exit(2)
		}
		id, err := server.CreateUser(ctx, username, password, true)
		if err != nil {
			logger.Error("create admin failed", "error", err)
			os.Exit(1)
		}
		logger.Info("admin created", "id", id, "username", username)
		return
	}

	httpServer := &http.Server{
		Addr: config.ListenAddress, Handler: server.Handler(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second,
	}
	shutdownContext, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownContext.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()
	logger.Info("TODO sync server listening", "address", config.ListenAddress)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}
