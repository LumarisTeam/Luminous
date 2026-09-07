package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"luminous/internal/config"
	"luminous/internal/handler"
	"luminous/internal/mcpserver"
	"luminous/internal/middleware"
	"luminous/internal/repository"
	"luminous/internal/router"

	"github.com/gin-gonic/gin"
)

func main() {
	slog.Info("Starting Luminous server")

	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	gin.SetMode(cfg.Server.Mode)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pgRepo, err := repository.NewPGSchoolRepository(ctx, cfg.Database)
	if err != nil {
		slog.Error("Failed to initialize PostgreSQL repository", "error", err)
		os.Exit(1)
	}
	defer pgRepo.Close()

	schoolHandler := handler.NewSchoolHandler(pgRepo)
	adminHandler := handler.NewAdminHandler(pgRepo)
	appHandler := handler.NewAppHandler(cfg.Release)
	mcpHandler := mcpserver.NewHandler(pgRepo, cfg.Release)

	r, err := router.SetupRouter(schoolHandler, adminHandler, appHandler,
		cfg.Auth.AdminToken, cfg.Server.CORSOrigin,
		cfg.RateLimit.Rate, cfg.RateLimit.Burst,
		cfg.Server.TrustedProxies, mcpHandler, cfg.Auth.MCPToken)
	if err != nil {
		slog.Error("Failed to setup router", "error", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:           addr,
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20,
		TLSConfig:      &tls.Config{MinVersion: tls.VersionTLS12},
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	go func() {
		srvCfg := cfg.Server
		if srvCfg.TLSCert != "" && srvCfg.TLSKey != "" {
			slog.Info("Server listening with TLS", "addr", addr)
			if err := srv.ListenAndServeTLS(srvCfg.TLSCert, srvCfg.TLSKey); err != nil && err != http.ErrServerClosed {
				slog.Error("Server failed", "error", err)
				os.Exit(1)
			}
		} else {
			slog.Info("Server listening", "addr", addr)
			slog.Warn("TLS not configured — use a reverse proxy for production")
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("Server failed", "error", err)
				os.Exit(1)
			}
		}
	}()

	<-quit
	slog.Info("Shutting down server...")
	middleware.StopRateLimiter()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("Server exited")
}
