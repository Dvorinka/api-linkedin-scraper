package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"apiservices/linkedin-scraper/internal/linkedin/api"
	"apiservices/linkedin-scraper/internal/linkedin/auth"
	"apiservices/linkedin-scraper/internal/linkedin/scrape"
)

func main() {
	logger := log.New(os.Stdout, "[linkedin] ", log.LstdFlags)

	port := envString("PORT", "8095")
	apiKey := envString("LINKEDIN_API_KEY", "dev-linkedin-key")
	if apiKey == "dev-linkedin-key" {
		logger.Println("LINKEDIN_API_KEY not set, using default development key")
	}
	baseURL := envString("LINKEDIN_BASE_URL", "https://www.linkedin.com")

	service := scrape.NewService(baseURL)
	handler := api.NewHandler(service)

	mux := http.NewServeMux()
	mux.Handle("/v1/linkedin/", auth.Middleware(apiKey)(handler))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadTimeout:       12 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Printf("service listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("shutdown error: %v", err)
	}
}

func envString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
