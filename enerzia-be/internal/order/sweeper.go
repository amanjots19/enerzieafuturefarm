package order

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// SweepStore is the persistence surface the Sweeper needs. *Repository
// satisfies it; tests substitute a stub.
type SweepStore interface {
	FindExpiredPending(ctx context.Context, now time.Time, limit int) ([]Order, error)
	MarkExpired(ctx context.Context, orderID string, now time.Time) (bool, error)
}

// SweeperConfig groups the dependencies injected into Sweeper.
type SweeperConfig struct {
	Repo      SweepStore
	Catalogue StockKeeper
	Logger    *slog.Logger
	// Batch is the maximum number of orders processed per tick.
	// Defaults to 100 when zero.
	Batch int
	// Now overrides the clock. Defaults to time.Now when nil.
	Now func() time.Time
}

// Sweeper finds abandoned pending_payment reservations and releases their
// stock. A goroutine on a ticker drives it in production; tests call
// SweepOnce directly.
type Sweeper struct {
	repo      SweepStore
	catalogue StockKeeper
	logger    *slog.Logger
	batch     int
	now       func() time.Time
}

// NewSweeper builds a Sweeper from its dependencies.
func NewSweeper(cfg SweeperConfig) *Sweeper {
	if cfg.Batch <= 0 {
		cfg.Batch = 100
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Sweeper{
		repo:      cfg.Repo,
		catalogue: cfg.Catalogue,
		logger:    cfg.Logger,
		batch:     cfg.Batch,
		now:       cfg.Now,
	}
}

// SweepOnce finds all pending_payment orders whose expiresAt is before now,
// claims each with a guarded updateOne, and releases stock only when the
// update reports modifiedCount == 1.
//
// The order of those two writes is the entire point. The guarded status
// update runs first; stock is returned only when it succeeds. A zero
// modifiedCount means another process — the callback, the webhook, or a
// concurrent sweeper — got there first. Releasing stock then would hand back
// units that a paid order already owns (schema.md §orders).
func (s *Sweeper) SweepOnce(ctx context.Context, now time.Time) (int, error) {
	orders, err := s.repo.FindExpiredPending(ctx, now, s.batch)
	if err != nil {
		return 0, fmt.Errorf("order: sweep: find expired: %w", err)
	}

	swept := 0
	for _, o := range orders {
		modified, err := s.repo.MarkExpired(ctx, o.OrderID, now)
		if err != nil {
			return swept, fmt.Errorf("order: sweep: mark expired: %w", err)
		}
		if !modified {
			continue // another process got there first — do NOT return stock
		}
		swept++
		s.sweepReleaseLines(ctx, o.Lines)
	}
	return swept, nil
}

// Run starts a ticker loop that calls SweepOnce on each tick until ctx is
// cancelled. A sweep failure is logged and the loop continues so one bad
// tick cannot kill the sweeper.
func (s *Sweeper) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := s.now()
			n, err := s.SweepOnce(ctx, now)
			if err != nil {
				s.logger.ErrorContext(ctx, "order: sweep: tick failed",
					slog.Any("error", err))
				continue
			}
			if n > 0 {
				s.logger.InfoContext(ctx, "order: sweep: expired reservations",
					slog.Int("count", n))
			}
		}
	}
}

// sweepReleaseLines returns each line's stock, logging but not propagating
// errors so a ReturnStock failure on one line does not abort the remaining
// lines.
func (s *Sweeper) sweepReleaseLines(ctx context.Context, lines []Line) {
	for _, l := range lines {
		if err := s.catalogue.ReturnStock(ctx, l.ProductID, l.Qty); err != nil {
			s.logger.ErrorContext(ctx, "order: sweep: return stock",
				slog.Any("error", err))
		}
	}
}
