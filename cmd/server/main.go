package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go-microservice/internal/config"
	"go-microservice/internal/handlers"
	"go-microservice/internal/middleware"
	"go-microservice/internal/repository"
	appTLS "go-microservice/internal/tls"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	cfg := config.Load()

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
	userHandler := handlers.NewUserHandler(userRepo, cfg)
	healthHandler := handlers.NewHealthHandler(mongoClient)

	// Router
	r := gin.Default()
	r.SetTrustedProxies(nil) //nolint:errcheck

	// Redirect HTTP → HTTPS (only in prod where we bind both ports)
	if cfg.Env == "production" {
		r.Use(func(c *gin.Context) {
			if c.Request.TLS == nil {
				url := "https://" + c.Request.Host + c.Request.RequestURI
				c.Redirect(http.StatusMovedPermanently, url)
				c.Abort()
				return
			}
			c.Next()
		})
	}

	// Public routes
	r.GET("/health", healthHandler.Health)
	r.POST("/auth/register", userHandler.Register)
	r.POST("/auth/login", userHandler.Login)

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

	// Build TLS config
	tlsCfg := appTLS.MustGetTLSConfig(&appTLS.Config{
		CertFile: cfg.TLSCert,
		KeyFile:  cfg.TLSKey,
		Env:      cfg.Env,
	})

	httpsPort := cfg.TLSPort
	srv := &http.Server{
		Addr:      ":" + httpsPort,
		Handler:   r,
		TLSConfig: tlsCfg,
	}

	// In production also spin up an HTTP server that redirects to HTTPS
	if cfg.Env == "production" {
		go func() {
			redirectSrv := &http.Server{
				Addr: ":80",
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, "https://"+r.Host+r.RequestURI, http.StatusMovedPermanently)
				}),
			}
			log.Println("HTTP redirect server listening on :80")
			if err := redirectSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTP redirect server error: %v", err)
			}
		}()
	}

	// Graceful shutdown
	go func() {
		log.Printf("HTTPS server starting on port %s (env=%s)", httpsPort, cfg.Env)
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

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
