package phase2

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
	"redis-timeout-tracking-poc/pkg/phase1"
)

type Service struct {
	config         *config.Config
	logger         *logrus.Logger
	metrics        *metrics.Metrics
	timeoutManager *phase1.TimeoutManager
	streamProducer *StreamProducer
	streamConsumer *StreamConsumer
	server         *http.Server
}

func NewService(rdb *redis.Client, config *config.Config, logger *logrus.Logger, metrics *metrics.Metrics) *Service {
	timeoutManager := phase1.NewTimeoutManager(rdb, config, logger, metrics)
	streamProducer := NewStreamProducer(rdb, config, logger, metrics)
	streamConsumer := NewStreamConsumer(rdb, config, logger, metrics)

	return &Service{
		config:         config,
		logger:         logger,
		metrics:        metrics,
		timeoutManager: timeoutManager,
		streamProducer: streamProducer,
		streamConsumer: streamConsumer,
	}
}

func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting Phase 2 timeout tracking service")

	// Start stream producer (handles leader election internally)
	if err := s.streamProducer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start stream producer: %w", err)
	}

	// Start stream consumer (all pods consume)
	if err := s.streamConsumer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start stream consumer: %w", err)
	}

	// Start HTTP server
	if err := s.startHTTPServer(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	s.logger.WithField("pod_id", s.config.PodID).Info("Phase 2 service started successfully")
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	s.logger.Info("Stopping Phase 2 service")

	// Stop stream components
	s.streamProducer.Stop()
	s.streamConsumer.Stop()

	// Stop HTTP server
	if s.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.WithError(err).Error("Failed to shutdown HTTP server gracefully")
			return err
		}
	}

	s.logger.Info("Phase 2 service stopped")
	return nil
}

func (s *Service) IsLeader() bool {
	return s.streamProducer.IsLeader()
}

func (s *Service) GetTimeoutManager() *phase1.TimeoutManager {
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
