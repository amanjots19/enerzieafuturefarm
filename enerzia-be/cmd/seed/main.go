// Command seed loads the catalogue into MongoDB.
//
// It is idempotent: running it against a populated database refreshes copy and
// prices rather than duplicating or deleting anything. Safe to re-run after
// every change to the seed tables in internal/catalogue/seed.go.
//
//	make seed
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/config"
	"github.com/enerzia/enerzia-be/internal/mongodb"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(logger); err != nil {
		logger.Error("seed failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoDB, cfg.MongoTimeout)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Disconnect(context.WithoutCancel(ctx)); err != nil {
			logger.Error("mongo disconnect", slog.Any("error", err))
		}
	}()

	repo := catalogue.NewRepository(client.DB())

	if err := repo.EnsureIndexes(ctx); err != nil {
		return err
	}

	products := catalogue.SeedProducts()
	tiles := catalogue.SeedTrustTiles()
	if err := repo.Seed(ctx, products, tiles); err != nil {
		return err
	}

	logger.Info("catalogue seeded",
		slog.String("database", cfg.MongoDB),
		slog.Int("products", len(products)),
		slog.Int("trustTiles", len(tiles)),
	)
	return nil
}
