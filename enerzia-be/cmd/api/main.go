// Command api runs the Enerzia shop HTTP API.
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

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/cart"
	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/mongodb"
	"github.com/enerzia/enerzia-be/internal/msg91"
	"github.com/enerzia/enerzia-be/internal/order"
	"github.com/enerzia/enerzia-be/internal/razorpay"
	"github.com/enerzia/enerzia-be/internal/server"
)

// version is the build identifier reported by /health. Override at build time
// with -ldflags "-X main.version=...".
var version = "0.1.0"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

// run holds the startup sequence so every failure returns an error rather than
// calling os.Exit from deep in the call stack.
func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	started := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mongoClient, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoDB, cfg.MongoTimeout)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownGrace)
		defer cancel()
		if err := mongoClient.Disconnect(shutdownCtx); err != nil {
			logger.Error("mongo disconnect", slog.Any("error", err))
		}
	}()

	catalogueRepo := catalogue.NewRepository(mongoClient.DB())
	if err := catalogueRepo.EnsureIndexes(ctx); err != nil {
		return err
	}

	authRepo := auth.NewRepository(mongoClient.DB())
	if err := authRepo.EnsureIndexes(ctx); err != nil {
		return err
	}

	// The development sender only logs the code, and devCode is only returned
	// outside production. Both are gated on the same flag so they cannot
	// disagree: production gets a real provider or a loud failure.
	var sender auth.Sender = auth.UnconfiguredSender{}
	if !cfg.IsProduction() {
		sender = auth.NewLogSender(logger)
	}

	// MSG91 verifier: required in production, optional elsewhere.
	// An empty key selects Unconfigured, which rejects every verification call.
	var verifier msg91.Verifier = msg91.Unconfigured{}
	if cfg.MSG91AuthKey != "" {
		verifier = msg91.NewClient(cfg.MSG91AuthKey)
	}

	authService := auth.NewService(auth.ServiceConfig{
		Store:      authRepo,
		Sender:     sender,
		Tokens:     auth.NewTokenIssuer([]byte(cfg.JWTSecret), auth.TokenTTL),
		Pepper:     []byte(cfg.JWTSecret),
		RevealCode: !cfg.IsProduction(),
		Verifier:   verifier,
	})

	cartService := cart.NewService(cart.NewRepository(mongoClient.DB()), catalogueRepo)

	orderRepo := order.NewRepository(mongoClient.DB())
	if err := orderRepo.EnsureIndexes(ctx); err != nil {
		return err
	}

	paymentEventsRepo := order.NewPaymentEventsRepository(mongoClient.DB())

	var gateway razorpay.Gateway = razorpay.Unconfigured{}
	if cfg.RazorpayKeyID != "" {
		gateway = razorpay.NewClient(cfg.RazorpayKeyID, cfg.RazorpayKeySecret, cfg.RazorpayWebhookSecret)
	}

	orderService := order.NewService(order.ServiceConfig{
		Repo:          orderRepo,
		Cart:          cartService,
		CartClearer:   cartService,
		Auth:          authService,
		Catalogue:     catalogueRepo,
		Gateway:       gateway,
		Events:        paymentEventsRepo,
		RazorpayKeyID: cfg.RazorpayKeyID,
		Logger:        logger,
	})

	sweeper := order.NewSweeper(order.SweeperConfig{
		Repo:      orderRepo,
		Catalogue: catalogueRepo,
		Logger:    logger,
		Batch:     100,
	})
	go sweeper.Run(ctx, cfg.SweeperInterval)

	srv := &http.Server{
		Addr: cfg.Addr(),
		Handler: server.New(server.Deps{
			Config:    cfg,
			Mongo:     mongoClient,
			Catalogue: catalogue.NewHandler(catalogue.NewService(catalogueRepo), logger),
			Auth:      auth.NewHandler(authService, logger),
			Cart:      cart.NewHandler(cartService, authService, logger),
			Order:     order.NewHandler(orderService, authService, logger),
			Logger:    logger,
			Version:   version,
			Started:   started,
		}),
		ReadHeaderTimeout: cfg.ReadTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api listening",
			slog.String("addr", cfg.Addr()),
			slog.String("env", string(cfg.AppEnv)),
			slog.String("version", version),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining",
			slog.Duration("grace", cfg.ShutdownGrace))
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	logger.Info("shutdown complete")
	return nil
}
