package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lsf-exporter/internal/collector"
	"lsf-exporter/internal/config"
	"lsf-exporter/internal/logger"
	"lsf-exporter/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(2)
	}

	log := logger.New(os.Stdout, cfg.LogLevel)

	src, err := collector.NewLSFSource(cfg.LSF)
	if err != nil {
		log.Error("failed to initialize LSF collector", "error", err)
		os.Exit(1)
	}
	defer src.Close()

	svc := collector.NewService(src, cfg.Collector, log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go svc.Run(ctx)

	mux := http.NewServeMux()
	server.Register(mux, svc, log)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting lsf exporter", "listen_address", cfg.ListenAddress, "collection_interval", cfg.Collector.Interval.String())
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown failed", "error", err)
		os.Exit(1)
	}
}
