package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"family-app-go/internal/app"
	"family-app-go/pkg/logger"
)

func main() {
	log := logger.NewFromEnv()
	log.Info("app: starting")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New(log)
	if err != nil {
		log.Critical("app: init failed", "err", err)
		os.Exit(1)
	}

	srv := application.HTTPServer()
	log.Info("http: listening", "addr", srv.Addr)

	serverErrCh := make(chan error, 1)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
		close(serverErrCh)
	}()

	exitCode := 0
	select {
	case <-ctx.Done():
		log.Info("app: shutdown signal received")
	case err := <-serverErrCh:
		if err != nil {
			log.Critical("http: server failed", "addr", srv.Addr, "err", err)
			exitCode = 1
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http: graceful shutdown failed", "err", err)
		exitCode = 1
	}

	if err := application.Close(); err != nil {
		log.Error("app: close failed", "err", err)
		exitCode = 1
	}

	if exitCode == 0 {
		log.Info("app: stopped")
		return
	}

	os.Exit(exitCode)
}
