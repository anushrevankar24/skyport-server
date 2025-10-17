package main

import (
	"log"
	"skyport-server/internal/config"
	"skyport-server/internal/database"
	"skyport-server/internal/handlers"
	"skyport-server/internal/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if it exists (optional)
	if err := godotenv.Load(".env"); err != nil {
		log.Println("No .env file found, using environment variables or defaults")
	}

	// Load configuration
	cfg := config.Load()

	// Initialize database
	db, err := database.Initialize(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Run migrations
	if err := database.RunMigrations(db); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}

	// Initialize router
	r := gin.Default()

	// CORS middleware
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{cfg.WebAppURL},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(db, cfg.JWTSecret)
	tunnelHandler := handlers.NewTunnelHandler(db)
	proxyHandler := handlers.NewProxyHandler(db, tunnelHandler)

	// Routes
	api := r.Group("/api/v1")
	{
		// Auth routes
		auth := api.Group("/auth")
		{
			auth.POST("/signup", authHandler.SignUp)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/agent-auth", authHandler.AgentAuth)
		}

		// Protected routes
		protected := api.Group("/")
		protected.Use(middleware.AuthMiddleware(cfg.JWTSecret))
		{
			protected.GET("/profile", authHandler.GetProfile)
			protected.GET("/tunnels", tunnelHandler.GetTunnels)
			protected.POST("/tunnels", tunnelHandler.CreateTunnel)
			protected.DELETE("/tunnels/:id", tunnelHandler.DeleteTunnel)
			protected.POST("/tunnels/:id/stop", tunnelHandler.StopTunnel)

			// Tunnel connection WebSocket
			protected.GET("/tunnel/connect", tunnelHandler.ConnectTunnel)
		}
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Subdomain proxy - catch all other routes for subdomain handling
	r.NoRoute(proxyHandler.HandleSubdomain)

	log.Printf("Server starting on port %s", cfg.Port)
	log.Fatal(r.Run(":" + cfg.Port))
}
