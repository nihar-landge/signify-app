package main

import (
	"log"
	"signify/internal/api"
	"signify/internal/repository"
	"signify/internal/service"
	"signify/internal/websocket"
	"signify/pkg/config"

	"github.com/gofiber/fiber/v2"
)

func main() {
	// Load Config
	cfg := config.Load()

	// Initialize Services
	streamProcessor := service.NewStreamProcessor(cfg)
	landmarkProcessor := service.NewLandmarkProcessor(cfg)

	// Initialize PostgreSQL (optional — graceful if unavailable)
	var postgresRepo *repository.PostgresRepository
	if cfg.PostgresURL != "" {
		var err error
		postgresRepo, err = repository.NewPostgresRepository(cfg.PostgresURL)
		if err != nil {
			log.Printf("Warning: PostgreSQL unavailable: %v (sessions will be in-memory only)", err)
		} else {
			defer postgresRepo.Close()
		}
	}

	// Initialize Session Manager with optional persistence
	sessionManager := service.NewSessionManager(postgresRepo)

	// Initialize Inference Client (connects to Python gRPC service)
	inferenceClient, err := service.NewInferenceClient(cfg.SignToTextURL)
	if err != nil {
		log.Printf("Warning: Failed to create inference client: %v", err)
	} else {
		log.Printf("Inference client connected to %s", cfg.SignToTextURL)
		defer inferenceClient.Close()
	}

	// Initialize WebSocket Hub
	hub := websocket.NewHub()
	go hub.Run()

	// Initialize API Handler (with session manager and postgres repo)
	handler := api.NewHandler(streamProcessor, hub, sessionManager, postgresRepo)

	// Initialize Call Handler for relay calls
	callHandler := api.NewCallHandler(landmarkProcessor, streamProcessor, inferenceClient, cfg)

	// Setup Fiber with configuration
	app := fiber.New(fiber.Config{
		AppName: "Signify Backend v1.0",
	})

	// Setup all routes (includes middleware)
	api.SetupRoutes(app, handler, callHandler)

	log.Printf("Server starting on port %s", cfg.Port)
	log.Printf("Relay call functionality enabled")
	log.Fatal(app.Listen(":" + cfg.Port))
}
