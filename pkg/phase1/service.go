package phase1

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"redis-timeout-tracking-poc/pkg/config"
	"redis-timeout-tracking-poc/pkg/metrics"
)

type Service struct {
	config         *config.Config
	logger         *logrus.Logger
	metrics        *metrics.Metrics
	timeoutManager *TimeoutManager
	leaderElection *LeaderElection
	server         *http.Server
}

func NewService(rdb *redis.Client, config *config.Config, logger *logrus.Logger, metrics *metrics.Metrics) *Service {
	timeoutManager := NewTimeoutManager(rdb, config, logger, metrics)
	leaderElection := NewLeaderElection(rdb, config, logger, metrics)

	return &Service{
		config:         config,
		logger:         logger,
		metrics:        metrics,
		timeoutManager: timeoutManager,
		leaderElection: leaderElection,
	}
}

func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting Phase 1 timeout tracking service")

	// Start leader election
	if err := s.leaderElection.Start(ctx); err != nil {
		return fmt.Errorf("failed to start leader election: %w", err)
	}

	// Start HTTP server
	if err := s.startHTTPServer(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start cleanup routine
	go s.cleanupRoutine(ctx)

	s.logger.WithField("pod_id", s.config.PodID).Info("Phase 1 service started successfully")
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	s.logger.Info("Stopping Phase 1 service")

	// Stop leader election
	s.leaderElection.Stop()

	// Stop HTTP server
	if s.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.WithError(err).Error("Failed to shutdown HTTP server gracefully")
			return err
		}
	}

	s.logger.Info("Phase 1 service stopped")
	return nil
}

func (s *Service) IsLeader() bool {
	return s.leaderElection.IsLeader()
}

func (s *Service) GetTimeoutManager() *TimeoutManager {
	return s.timeoutManager
}

func (s *Service) startHTTPServer(ctx context.Context) error {
	s.server = s.createHTTPServer()

	go func() {
		s.logger.WithField("port", s.config.Port).Info("Starting HTTP server")
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.WithError(err).Fatal("HTTP server failed")
		}
	}()

	return nil
}

func (s *Service) createHTTPServer() *http.Server {
	router := mux.NewRouter()

	// API routes
	router.HandleFunc("/conversations/{id}/agent-message", s.handleAgentMessage).Methods("POST")
	router.HandleFunc("/conversations/{id}/customer-response", s.handleCustomerResponse).Methods("POST")
	router.HandleFunc("/health", s.handleHealth).Methods("GET")
	router.HandleFunc("/status", s.handleStatus).Methods("GET")

	// Metrics endpoint
	router.Handle("/metrics", promhttp.Handler()).Methods("GET")

	return &http.Server{
		Addr:         ":" + s.config.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func (s *Service) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour) // Cleanup every hour
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.leaderElection.IsLeader() {
				// Clean up conversations older than 24 hours
				maxAge := 24 * time.Hour
				if err := s.timeoutManager.CleanupExpiredConversations(ctx, maxAge); err != nil {
					s.logger.WithError(err).Error("Failed to cleanup expired conversations")
				}
			}
		}
	}
}
