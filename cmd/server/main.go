// @title           go-microservice API
// @version         1.0
// @description     A production-ready user management REST API with JWT authentication.

// @contact.name    API Support
// @contact.email   support@example.com

// @license.name    MIT
// @license.url     https://opensource.org/licenses/MIT

// @host            localhost:8443
// @BasePath        /

// @schemes         https

// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 Enter your JWT token as: Bearer <token>
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go-microservice/docs"
	"go-microservice/internal/config"
	"go-microservice/internal/handlers"
	"go-microservice/internal/mail"
	"go-microservice/internal/middleware"
	"go-microservice/internal/observability"
	"go-microservice/internal/repository"
	appTLS "go-microservice/internal/tls"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/time/rate"
)

func main() {
	cfg := config.Load()

	logger := observability.NewLogger(cfg.LogLevel, cfg.LogJSON)
	slog.SetDefault(logger)

	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	if cfg.ServeHTTPOnly() {
		docs.SwaggerInfo.Host = "localhost:" + cfg.Port
		docs.SwaggerInfo.Schemes = []string{"http"}
	} else {
		docs.SwaggerInfo.Host = "localhost:" + cfg.TLSPort
		docs.SwaggerInfo.Schemes = []string{"https"}
	}

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		slog.Error("failed to connect to MongoDB", slog.Any("err", err))
		os.Exit(1)
	}
	defer mongoClient.Disconnect(context.Background()) //nolint:errcheck

	if err := mongoClient.Ping(ctx, nil); err != nil {
		slog.Error("MongoDB ping failed", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info("connected to MongoDB")

	db := mongoClient.Database(cfg.MongoDB)

	// Wire up dependencies
	userRepo := repository.NewUserRepository(db)
	refreshRepo := repository.NewRefreshTokenRepository(db)
	mailSender := mail.NewSender(cfg)
	userHandler := handlers.NewUserHandler(userRepo, refreshRepo, cfg, mailSender)
	healthHandler := handlers.NewHealthHandler(mongoClient)

	// Router (no gin.Default logger; structured access log + slog recovery)
	r := gin.New()
	r.SetTrustedProxies(nil) //nolint:errcheck
	r.Use(middleware.SlogRecovery(logger))
	r.Use(middleware.RequestID())
	if cfg.MetricsEnabled {
		r.Use(observability.PrometheusMiddleware())
	}
	r.Use(middleware.AccessLog(logger))

	if len(cfg.CORSAllowedOrigins) > 0 {
		r.Use(cors.New(cors.Config{
			AllowOrigins:     cfg.CORSAllowedOrigins,
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"},
			ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
			AllowCredentials: false,
			MaxAge:           86400,
		}))
	} else if cfg.IsProduction() {
		slog.Warn("CORS_ALLOWED_ORIGINS not set — cross-origin browser requests to this API will fail until you set it (e.g. https://app.example.com)")
	}

	// Swagger UI (disable in production)
	if !cfg.IsProduction() {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "https://localhost:"+cfg.TLSPort+"/swagger/index.html")
		})
		slog.Info("swagger UI available", slog.String("url", "https://localhost:"+cfg.TLSPort+"/swagger/index.html"))
	}

	// Public routes
	r.GET("/health", healthHandler.Health)
	if cfg.MetricsEnabled {
		r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	}

	// Separate buckets so a login flood (e.g. integration tests) does not block registration.
	registerLimit := middleware.PerIPRateLimit(rate.Every(2*time.Second), 15)
	loginLimit := middleware.PerIPRateLimit(rate.Every(2*time.Second), 15)
	forgotLimit := middleware.PerIPRateLimit(rate.Every(1*time.Minute), 8)
	resendVerifyLimit := middleware.PerIPRateLimit(rate.Every(1*time.Minute), 8)
	resetLimit := middleware.PerIPRateLimit(rate.Every(1*time.Second), 30)
	refreshLimit := middleware.PerIPRateLimit(rate.Every(2*time.Second), 30)
	logoutLimit := middleware.PerIPRateLimit(rate.Every(2*time.Second), 30)
	r.POST("/auth/register", registerLimit, userHandler.Register)
	r.POST("/auth/login", loginLimit, userHandler.Login)
	r.POST("/auth/refresh", refreshLimit, userHandler.Refresh)
	r.POST("/auth/logout", logoutLimit, userHandler.Logout)
	r.POST("/auth/forgot-password", forgotLimit, userHandler.ForgotPassword)
	r.POST("/auth/reset-password", resetLimit, userHandler.ResetPassword)
	r.POST("/auth/verify-email", resetLimit, userHandler.VerifyEmail)
	r.POST("/auth/resend-verification", resendVerifyLimit, userHandler.ResendVerification)

	// Protected routes
	protected := r.Group("/")
	protected.Use(middleware.AuthRequired(cfg.JWTSecret))
	{
		protected.GET("/me", userHandler.GetMe)
		protected.PUT("/users/:id", userHandler.UpdateUser)

		// Admin-only routes
		admin := protected.Group("/")
		admin.Use(middleware.AdminOnly())
		{
			admin.GET("/users", userHandler.ListUsers)
			admin.GET("/users/:id", userHandler.GetUser)
			admin.DELETE("/users/:id", userHandler.DeleteUser)
		}
	}

	var srv *http.Server

	if cfg.ServeHTTPOnly() {
		addr := ":" + cfg.Port
		srv = &http.Server{
			Addr:    addr,
			Handler: r,
		}
		go func() {
			slog.Info("HTTP server starting", slog.String("addr", addr), slog.String("env", cfg.Env))
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("listen", slog.Any("err", err))
				os.Exit(1)
			}
		}()
	} else {
		tlsCfg := appTLS.MustGetTLSConfig(&appTLS.Config{
			CertFile: cfg.TLSCert,
			KeyFile:  cfg.TLSKey,
			Env:      cfg.Env,
		})

		httpsPort := cfg.TLSPort
		srv = &http.Server{
			Addr:      ":" + httpsPort,
			Handler:   r,
			TLSConfig: tlsCfg,
		}
		go func() {
			slog.Info("HTTPS server starting", slog.String("port", httpsPort), slog.String("env", cfg.Env))
			if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				slog.Error("listen", slog.Any("err", err))
				os.Exit(1)
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced shutdown", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info("server exited")
}
