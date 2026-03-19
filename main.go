package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/user-service/internal/config"
	"github.com/example/user-service/internal/handlers"
	"github.com/example/user-service/internal/middleware"
	"github.com/example/user-service/internal/repository"
	"github.com/gin-gonic/gin"
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

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Server starting on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
