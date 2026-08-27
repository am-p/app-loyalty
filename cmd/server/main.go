package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/handler"
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	poolConfig, err := cfg.PoolConfig()
	if err != nil {
		logger.Error("invalid database configuration", "error", err)
		os.Exit(1)
	}
	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := pgxpool.NewWithConfig(startupCtx, poolConfig)
	cancel()
	if err != nil {
		logger.Error("database pool failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	repo := repository.New(pool)
	tokens := auth.NewTokens(cfg.JWTSecret, cfg.JWTIssuer)
	svc := service.New(repo, tokens, cfg)
	h := &handler.Handler{Service: svc, Repo: repo, Limiter: middleware.NewRateLimiter(), Logger: logger, TrustedProxyCount: cfg.TrustedProxyCount}
	router := newRouter(h, tokens, logger)
	server := &http.Server{Addr: ":" + cfg.Port, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout, MaxHeaderBytes: 32 << 10}
	go func() {
		logger.Info("server starting", "port", cfg.Port, "version", cfg.AppVersion)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}

func newRouter(h *handler.Handler, tokens *auth.Tokens, logger *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.RequestContext(logger), middleware.Timeout(12*time.Second), middleware.Recovery(logger), middleware.CORS())
	r.NoRoute(func(c *gin.Context) { web.Error(c, http.StatusNotFound, "NOT_FOUND", "Recurso no encontrado", nil) })
	v1 := r.Group("/v1")
	v1.GET("/health/live", h.HealthLive)
	v1.GET("/health/ready", h.HealthReady)
	v1.GET("/version", h.Version)
	v1.POST("/auth/register", h.RegisterCustomer)
	v1.POST("/auth/login", h.Login)
	v1.POST("/demo/comercios", h.RegisterDemoMerchant)
	authenticated := v1.Group("")
	authenticated.Use(middleware.RequireAuth(tokens, h.Repo))
	authenticated.GET("/me", h.Me)
	authenticated.GET("/marcas", h.ListBrands)
	authenticated.GET("/marcas/:brand_id", h.Brand)
	authenticated.GET("/marcas/:brand_id/movimientos", h.BrandMovements)
	authenticated.GET("/marcas/:brand_id/clientes", h.BrandCustomers)
	authenticated.GET("/marcas/:brand_id/metricas/resumen", h.BrandMetrics)
	authenticated.GET("/clientes/me", h.Customer)
	authenticated.GET("/clientes/me/tarjetas", h.Cards)
	authenticated.GET("/clientes/me/tarjetas/:card_id/movimientos", h.CardMovements)
	authenticated.POST("/movimientos/preview", h.Preview)
	authenticated.POST("/movimientos/scan", h.ConfirmAccumulation)
	authenticated.POST("/movimientos/canje", h.ConfirmRedemption)
	authenticated.GET("/movimientos/idempotencia/:idempotency_key", h.Idempotency)
	r.POST("/auth/register", h.LegacyRegister)
	r.POST("/auth/login", h.LegacyLogin)
	r.POST("/auth/google", h.LegacyGoogle)
	r.GET("/me", middleware.RequireAuth(tokens, h.Repo), h.LegacyMe)
	return r
}
