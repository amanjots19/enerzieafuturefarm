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
	"flag"
	"fmt"
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
	overwrite := flag.Bool("overwrite", false,
		"reset existing products to code values, discarding admin edits (stock and images are kept)")
	flag.Parse()
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	// The live catalogue belongs to the catalogue manager, who decides what the
	// shop sells through the admin console. This command is a development
	// bootstrap for an empty database — it has no business deciding that in
	// production, in either direction: creating a product puts something on sale
	// that nobody chose to sell, and --overwrite would revert prices and copy.
	//
	// Refused outright rather than gated behind another flag. An escape hatch
	// here just recreates the hazard at one remove; if this is ever genuinely
	// needed against production it should cost a deliberate code change.
	if cfg.AppEnv == config.EnvProduction {
		return fmt.Errorf(
			"refusing to seed a production database: the catalogue is maintained " +
				"through the admin console, add or edit products there")
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

	// Default is insert-only: existing products are the administrator's, edited
	// through the console, and a routine re-seed must not revert their prices,
	// copy or retired flags. --overwrite is the deliberate escape hatch that
	// resets them to the values in code (stock and images always survive).
	if *overwrite {
		logger.Warn("catalogue seed: OVERWRITE — resetting existing products to code values")
		if err := repo.SeedOverwrite(ctx, products, tiles); err != nil {
			return err
		}
	} else if err := repo.Seed(ctx, products, tiles); err != nil {
		return err
	}

	logger.Info("catalogue seeded",
		slog.String("database", cfg.MongoDB),
		slog.Bool("overwrite", *overwrite),
		slog.Int("products", len(products)),
		slog.Int("trustTiles", len(tiles)),
	)
	return nil
}
