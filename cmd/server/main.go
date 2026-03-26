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
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go-microservice/docs"
	"go-microservice/internal/config"
	"go-microservice/internal/handlers"
	"go-microservice/internal/mail"
	"go-microservice/internal/middleware"
	"go-microservice/internal/repository"
	appTLS "go-microservice/internal/tls"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/time/rate"
)

func main() {
	cfg := config.Load()

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
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(context.Background()) //nolint:errcheck

	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Fatalf("MongoDB ping failed: %v", err)
	}
	log.Println("Connected to MongoDB")

	db := mongoClient.Database(cfg.MongoDB)

	// Wire up dependencies
	userRepo := repository.NewUserRepository(db)
	mailSender := mail.NewSender(cfg)
	userHandler := handlers.NewUserHandler(userRepo, cfg, mailSender)
	healthHandler := handlers.NewHealthHandler(mongoClient)

	// Router
	r := gin.Default()
	r.SetTrustedProxies(nil) //nolint:errcheck

	if len(cfg.CORSAllowedOrigins) > 0 {
		r.Use(cors.New(cors.Config{
			AllowOrigins:     cfg.CORSAllowedOrigins,
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
			ExposeHeaders:    []string{"Content-Length"},
			AllowCredentials: false,
			MaxAge:           86400,
		}))
	} else if cfg.IsProduction() {
		log.Println("CORS_ALLOWED_ORIGINS not set — cross-origin browser requests to this API will fail until you set it (e.g. https://app.example.com)")
	}

	// Swagger UI (disable in production)
	if !cfg.IsProduction() {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "https://localhost:"+cfg.TLSPort+"/swagger/index.html")
		})
		log.Printf("Swagger UI at https://localhost:%s/swagger/index.html (adjust scheme/port if using TLS offloading)", cfg.TLSPort)
	}

	// Public routes
	r.GET("/health", healthHandler.Health)

	// Separate buckets so a login flood (e.g. integration tests) does not block registration.
	registerLimit := middleware.PerIPRateLimit(rate.Every(2*time.Second), 15)
	loginLimit := middleware.PerIPRateLimit(rate.Every(2*time.Second), 15)
	forgotLimit := middleware.PerIPRateLimit(rate.Every(1*time.Minute), 8)
	resetLimit := middleware.PerIPRateLimit(rate.Every(1*time.Second), 30)
	r.POST("/auth/register", registerLimit, userHandler.Register)
	r.POST("/auth/login", loginLimit, userHandler.Login)
	r.POST("/auth/forgot-password", forgotLimit, userHandler.ForgotPassword)
	r.POST("/auth/reset-password", resetLimit, userHandler.ResetPassword)

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
			log.Printf("HTTP server starting on %s (env=%s, TLS at reverse proxy)", addr, cfg.Env)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen: %v", err)
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
			log.Printf("HTTPS server starting on port %s (env=%s)", httpsPort, cfg.Env)
			if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen: %v", err)
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	log.Println("Server exited")
}
