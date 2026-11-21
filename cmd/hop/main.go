// Command hop is a small link shortener built to be operated on Kubernetes.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bernisnukic/hop/internal/server"
	"github.com/bernisnukic/hop/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	listen := env("HOP_LISTEN", ":8080")
	metricsListen := env("HOP_METRICS_LISTEN", ":9090")
	baseURL := env("HOP_BASE_URL", "http://localhost:8080")
	// How long to keep serving after SIGTERM while readiness reports
	// "draining", so endpoints are removed before connections are refused.
	drainDelay := envDuration("HOP_DRAIN_DELAY", 5*time.Second)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metrics := server.NewMetrics(reg)

	var (
		st      store.Store
		migrate func(context.Context) error
	)
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		pg, err := store.OpenPostgres(ctx, dbURL)
		if err != nil {
			return err
		}
		defer pg.Close()
		st, migrate = pg, pg.Migrate
	} else if os.Getenv("HOP_STORE") == "memory" {
		log.Warn("using the in-memory store: links are lost on restart")
		st, migrate = store.NewMemory(), func(context.Context) error { return nil }
	} else {
		return errors.New("set DATABASE_URL, or HOP_STORE=memory for a throwaway store")
	}

	srv := server.New(st, metrics, log, baseURL)
	app := &http.Server{Addr: listen, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	ops := &http.Server{Addr: metricsListen, Handler: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), ReadHeaderTimeout: 5 * time.Second}

	errc := make(chan error, 2)
	for _, s := range []*http.Server{app, ops} {
		go func() {
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}()
	}
	log.Info("listening", "addr", listen, "metrics", metricsListen)

	// The database may still be starting (a new cluster, a failover), so
	// migrate with backoff while liveness already answers.
	go func() {
		backoff := time.Second
		for {
			err := migrate(ctx)
			if err == nil {
				srv.SetReady()
				log.Info("store ready")
				return
			}
			log.Warn("store not ready, retrying", "err", err, "in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 15*time.Second)
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info("draining", "delay", drainDelay)
	srv.Drain()
	time.Sleep(drainDelay)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := app.Shutdown(shutdownCtx)
	_ = ops.Shutdown(shutdownCtx)
	log.Info("stopped")
	return err
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
