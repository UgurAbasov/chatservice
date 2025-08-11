package main

import (
	"log"

	"chatservice/config"
	postgres "chatservice/internal/repository"
	
	http_delivery "chatservice/internal/delivery/http"
	ws_delivery "chatservice/internal/delivery/websocket"
	"chatservice/internal/middleware"
	"chatservice/internal/usecase"

	"github.com/gin-gonic/gin"
)

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func main() {
	cfg := config.Load()

	dbPool, err := postgres.NewDBPool(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Could not connect to the database: %v", err)
	}
	defer dbPool.Close()

	appRepo := postgres.NewAppRepository(dbPool)

	hub := ws_delivery.NewHub(appRepo)
	go hub.Run()

	appUsecase := usecase.NewAppUsecase(appRepo, hub, dbPool)

	concreteUsecase, ok := appUsecase.(*usecase.AppUsecase)
	if !ok {
		log.Fatal("Could not assert AppUsecase interface to concrete type *usecase.AppUsecase")
	}
	hub.SetUsecase(concreteUsecase)

	router := gin.Default()

	router.Use(CORSMiddleware())

	authMiddleware := middleware.AuthMiddleware(cfg.AuthServiceURL)
	router.Use(authMiddleware)

	http_delivery.RegisterRoutes(&router.RouterGroup, appUsecase)

	wsGroup := router.Group("/ws")
	wsGroup.GET("", ws_delivery.ServeWs(hub))

	log.Printf("Server starting on port %s", cfg.ServerPort)
	if err := router.Run(cfg.ServerPort); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}