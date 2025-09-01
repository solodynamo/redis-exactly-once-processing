package server

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"redis-timeout-tracking-poc/pkg/config"
	"redis-timeout-tracking-poc/pkg/handlers"
	"redis-timeout-tracking-poc/pkg/phase1"
)

func NewHTTPServer(config *config.Config, timeoutManager *phase1.TimeoutManager, logger *logrus.Logger, isLeaderFunc func() bool) *http.Server {
	handler := handlers.NewHandler(timeoutManager, logger, isLeaderFunc)

	router := mux.NewRouter()

	// API routes
	router.HandleFunc("/conversations/{id}/agent-message", handler.AgentMessage).Methods("POST")
	router.HandleFunc("/conversations/{id}/customer-response", handler.CustomerResponse).Methods("POST")
	router.HandleFunc("/health", handler.Health).Methods("GET")
	router.HandleFunc("/status", handler.Status).Methods("GET")

	// Metrics endpoint
	router.Handle("/metrics", promhttp.Handler()).Methods("GET")

	// Add logging middleware
	router.Use(loggingMiddleware(logger))

	return &http.Server{
		Addr:         ":" + config.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func loggingMiddleware(logger *logrus.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			next.ServeHTTP(w, r)

			logger.WithFields(logrus.Fields{
				"method":   r.Method,
				"path":     r.URL.Path,
				"duration": time.Since(start),
				"remote":   r.RemoteAddr,
			}).Debug("HTTP request processed")
		})
	}
}
