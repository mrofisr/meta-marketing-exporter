package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"meta-marketing-exporter/internal/config"
	"meta-marketing-exporter/internal/meta"
	"meta-marketing-exporter/internal/otel"
)

func main() {
	configPath := flag.String("config", "./config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}

	// Set up structured logging
	logLevel := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	slog.Info("starting meta-ads-exporter",
		"account_ids", cfg.AccountIDs,
		"poll_interval", cfg.PollInterval,
		"health_port", cfg.HealthPort)

	// Create Meta API client
	client := meta.NewClient(cfg.AccessToken)

	// Create OpenTelemetry meter provider
	ctx := context.Background()
	provider, err := otel.NewMeterProvider(ctx, cfg.OTLPEndpoint, cfg.AuthHeader)
	if err != nil {
		slog.Error("failed to create meter provider", "error", err)
		os.Exit(1)
	}

	meter := provider.Meter("meta-ads-exporter")
	gauges, err := otel.NewGaugeInstruments(meter)
	if err != nil {
		slog.Error("failed to create gauge instruments", "error", err)
		os.Exit(1)
	}

	store := otel.NewMetricsStore()
	registration, err := otel.RegisterGaugeCallbacks(meter, gauges, store)
	if err != nil {
		slog.Error("failed to register gauge callbacks", "error", err)
		os.Exit(1)
	}
	defer registration.Unregister()

	// Readiness tracking
	var ready atomic.Bool
	var consecutiveFailures atomic.Int32

	// Health endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready.Load() && consecutiveFailures.Load() < 3 {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	healthServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HealthPort),
		Handler: mux,
	}

	go func() {
		slog.Info("starting health server", "port", cfg.HealthPort)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health server error", "error", err)
		}
	}()

	// Graceful shutdown handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		slog.Info("shutdown signal received")
		cancel()
	}()

	// Resolve target account IDs
	var targetAccountIDs []string
	if cfg.IsAllAccounts() {
		slog.Info("discovering all active ad accounts")
		accounts, err := meta.ListAdAccounts(ctx, client)
		if err != nil {
			slog.Error("failed to list ad accounts", "error", err)
			os.Exit(1)
		}
		for _, acc := range accounts {
			if acc.AccountStatus == 1 {
				targetAccountIDs = append(targetAccountIDs, acc.ID)
			}
		}
		if len(targetAccountIDs) == 0 {
			slog.Warn("no active ad accounts found")
		}
		slog.Info("discovered active ad accounts", "count", len(targetAccountIDs), "accounts", targetAccountIDs)
	} else {
		targetAccountIDs = cfg.AccountIDs
	}

	// Poll loop
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	pollOnce := func() error {
		return meta.WithBackoff(ctx, func() error {
			var allMetrics []meta.CampaignMetrics
			for _, accountID := range targetAccountIDs {
				rows, err := meta.FetchTodayInsights(ctx, client, accountID)
				if err != nil {
					return fmt.Errorf("account %s: %w", accountID, err)
				}
				for _, row := range rows {
					allMetrics = append(allMetrics, meta.ExtractMetrics(row))
				}
			}

			store.Update(allMetrics)
			slog.Debug("poll completed", "campaigns", len(allMetrics), "accounts", len(targetAccountIDs))
			return nil
		})
	}

	// Initial poll before starting ticker
	slog.Info("starting initial poll")
	if err := pollOnce(); err != nil {
		var metaErr *meta.MetaAPIError
		if errors.As(err, &metaErr) && (metaErr.Code == 190 || metaErr.Code == 102) {
			slog.Error("authentication failed - token expired or invalid", "code", metaErr.Code, "message", metaErr.Message)
			os.Exit(1)
		}
		slog.Error("initial poll failed, will retry", "error", err)
		consecutiveFailures.Add(1)
	} else {
		slog.Info("initial poll succeeded")
		ready.Store(true)
		consecutiveFailures.Store(0)
	}

	// Main poll loop
	for {
		select {
		case <-ticker.C:
			if err := pollOnce(); err != nil {
				var metaErr *meta.MetaAPIError
				if errors.As(err, &metaErr) && (metaErr.Code == 190 || metaErr.Code == 102) {
					slog.Error("authentication failed - token expired or invalid", "code", metaErr.Code, "message", metaErr.Message)
					cancel()
					os.Exit(1)
				}
				slog.Error("poll failed", "error", err)
				consecutiveFailures.Add(1)
			} else {
				consecutiveFailures.Store(0)
			}

		case <-ctx.Done():
			slog.Info("shutting down gracefully")

			// Shutdown with 30s timeout
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer shutdownCancel()

			// Flush metrics
			if err := provider.Shutdown(shutdownCtx); err != nil {
				slog.Error("meter provider shutdown error", "error", err)
			}

			// Close health server
			if err := healthServer.Shutdown(shutdownCtx); err != nil {
				slog.Error("health server shutdown error", "error", err)
			}

			slog.Info("shutdown complete")
			return
		}
	}
}
