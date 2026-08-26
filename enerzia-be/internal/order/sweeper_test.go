package order_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/catalogue"
	"github.com/enerzia/enerzia-be/internal/order"
)

/* ================================================= fakeSweepStore */

// fakeSweepStore is an in-memory SweepStore for sweeper tests. Every call to
// MarkExpired records the orderID and returns the preset markModified flag,
// allowing tests to assert which orders were claimed and whether stock was
// released.
type fakeSweepStore struct {
	orders       []order.Order
	findErr      error
	markModified bool
	markErr      error
	markedIDs    []string
}

func (f *fakeSweepStore) FindExpiredPending(_ context.Context, _ time.Time, _ int) ([]order.Order, error) {
	return f.orders, f.findErr
}

func (f *fakeSweepStore) MarkExpired(_ context.Context, orderID string, _ time.Time) (bool, error) {
	if f.markErr != nil {
		return false, f.markErr
	}
	f.markedIDs = append(f.markedIDs, orderID)
	return f.markModified, nil
}

/* ================================================= helpers */

func newSweeper(repo order.SweepStore, stock *fakeStock) *order.Sweeper {
	return order.NewSweeper(order.SweeperConfig{
		Repo:      repo,
		Catalogue: stock,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Batch:     100,
	})
}

func testLine(productID catalogue.ID, qty int) order.Line {
	return order.Line{ProductID: productID, Qty: qty, Name: string(productID)}
}

/* ================================================= tests */

func TestSweepOnceEmpty(t *testing.T) {
	sw := newSweeper(&fakeSweepStore{}, newFakeStock())
	n, err := sw.SweepOnce(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("SweepOnce() error = %v, want nil", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
}

func TestSweepOnceModifiedCountZeroNoStockReturn(t *testing.T) {
	// THE critical test: when the guarded update reports modifiedCount == 0,
	// no stock may be returned. Releasing then would hand back units that a
	// paid order already owns (schema.md §orders, "The sweep uses the same
	// guard-in-the-filter idiom...").
	stock := newFakeStock()
	repo := &fakeSweepStore{
		orders: []order.Order{{
			OrderID: "EFF-000001",
			Lines:   []order.Line{testLine("tablets-120", 2)},
		}},
		markModified: false, // guard rejected — another process got there first
	}

	sw := newSweeper(repo, stock)
	n, err := sw.SweepOnce(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("SweepOnce() error = %v, want nil", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0 — sweeper must not count an order it did not own", n)
	}
	if stock.returned != 0 {
		t.Errorf("ReturnStock called %d times, want 0 — must not release stock for an unowned expiry",
			stock.returned)
	}
}

func TestSweepOnceModifiedCountOneReleasesAllLines(t *testing.T) {
	stock := newFakeStock()
	repo := &fakeSweepStore{
		orders: []order.Order{{
			OrderID: "EFF-000001",
			Lines: []order.Line{
				testLine("tablets-120", 2),
				testLine("powder-200", 1),
			},
		}},
		markModified: true,
	}

	sw := newSweeper(repo, stock)
	n, err := sw.SweepOnce(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("SweepOnce() error = %v, want nil", err)
	}
	if n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
	if stock.returned != 2 {
		t.Errorf("ReturnStock called %d times, want 2 (one per line)", stock.returned)
	}
}

func TestSweepOnceReturnStockFailureContinuesToNextLine(t *testing.T) {
	// A ReturnStock failure on one line must not abort the remaining lines.
	// Partial compensation is better than no compensation.
	stock := &fakeStock{failOnIdx: -1, returnErr: errors.New("db error")}
	repo := &fakeSweepStore{
		orders: []order.Order{{
			OrderID: "EFF-000001",
			Lines: []order.Line{
				testLine("tablets-120", 1),
				testLine("powder-200", 3),
			},
		}},
		markModified: true,
	}

	sw := newSweeper(repo, stock)
	_, err := sw.SweepOnce(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("SweepOnce() error = %v, want nil (ReturnStock failures are logged, not surfaced)", err)
	}
	if stock.returned != 2 {
		t.Errorf("ReturnStock called %d times, want 2 — failure on one line must not skip the others",
			stock.returned)
	}
}

func TestSweepOnceMultipleOrders(t *testing.T) {
	stock := newFakeStock()
	repo := &fakeSweepStore{
		orders: []order.Order{
			{OrderID: "EFF-000001", Lines: []order.Line{testLine("tablets-120", 1)}},
			{OrderID: "EFF-000002", Lines: []order.Line{testLine("powder-200", 2)}},
		},
		markModified: true,
	}

	sw := newSweeper(repo, stock)
	n, err := sw.SweepOnce(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("SweepOnce() error = %v, want nil", err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
	if stock.returned != 2 {
		t.Errorf("ReturnStock called %d times, want 2", stock.returned)
	}
	if len(repo.markedIDs) != 2 {
		t.Errorf("marked %d orders expired, want 2", len(repo.markedIDs))
	}
}

func TestNewSweeperDefaultsBatch(t *testing.T) {
	// Batch: 0 must not panic — NewSweeper defaults it to 100 internally.
	sw := order.NewSweeper(order.SweeperConfig{
		Repo:      &fakeSweepStore{},
		Catalogue: newFakeStock(),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	n, err := sw.SweepOnce(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("SweepOnce() error = %v, want nil", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
}

func TestSweeperRunStopsOnContextCancel(t *testing.T) {
	// Run must return promptly once the context is cancelled — it is the
	// shutdown signal that stops the ticker loop.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	repo := &fakeSweepStore{markModified: true, orders: []order.Order{
		{OrderID: "EFF-000001", Lines: []order.Line{testLine("tablets-120", 1)}},
	}}
	stock := newFakeStock()
	sw := order.NewSweeper(order.SweeperConfig{
		Repo:      repo,
		Catalogue: stock,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Batch:     100,
	})

	// Run blocks until ctx times out — with a 10 ms tick and a 50 ms timeout
	// at least one sweep fires before it returns.
	sw.Run(ctx, 10*time.Millisecond)

	if len(repo.markedIDs) == 0 {
		t.Error("Run: expected at least one sweep tick to fire before context was cancelled")
	}
}

func TestSweeperRunLogsErrorAndContinues(t *testing.T) {
	// A sweep that errors on every tick must not kill the loop; Run returns
	// only when the context is cancelled.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	repo := &fakeSweepStore{findErr: errors.New("mongo: connection reset")}
	sw := order.NewSweeper(order.SweeperConfig{
		Repo:      repo,
		Catalogue: newFakeStock(),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Batch:     100,
	})

	// If Run panics or hangs, the context timeout causes t.Context() to
	// expire and the test fails via the parent deadline.
	sw.Run(ctx, 10*time.Millisecond)
}

func TestSweepOnceFindErrorSurfaces(t *testing.T) {
	dbErr := errors.New("mongo: connection reset")
	repo := &fakeSweepStore{findErr: dbErr}

	sw := newSweeper(repo, newFakeStock())
	_, err := sw.SweepOnce(t.Context(), time.Now())
	if err == nil {
		t.Fatal("SweepOnce() error = nil, want a database error to surface")
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("error = %v, want it to wrap the original db error", err)
	}
}

func TestSweepOnceMarkErrorSurfaces(t *testing.T) {
	dbErr := errors.New("mongo: write conflict")
	repo := &fakeSweepStore{
		orders:  []order.Order{{OrderID: "EFF-000001"}},
		markErr: dbErr,
	}

	sw := newSweeper(repo, newFakeStock())
	_, err := sw.SweepOnce(t.Context(), time.Now())
	if err == nil {
		t.Fatal("SweepOnce() error = nil, want a database error to surface")
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("error = %v, want it to wrap the original db error", err)
	}
}

func TestSweeperRunDisabledByNonPositiveInterval(t *testing.T) {
	// The safety switch. A non-positive interval must return IMMEDIATELY and
	// write nothing — this is what lets a local process run against a database
	// whose data must not change.
	//
	// The uncancelled context is the assertion that matters. Run is given a
	// context that never expires, so if the guard is ever removed this test
	// does not fail with a wrong value, it HANGS on the ticker loop until the
	// suite times out. A cancelled context would let a broken Run return and
	// pass by accident.
	for _, interval := range []time.Duration{0, -time.Second} {
		repo := &fakeSweepStore{markModified: true, orders: []order.Order{
			{OrderID: "EFF-000001", Lines: []order.Line{testLine("tablets-120", 1)}},
		}}
		stock := newFakeStock()
		sw := newSweeper(repo, stock)

		sw.Run(context.Background(), interval)

		if len(repo.markedIDs) != 0 {
			t.Errorf("Run(%s): expected no orders claimed, got %v", interval, repo.markedIDs)
		}
		if stock.returned != 0 {
			t.Errorf("Run(%s): expected no stock returned, got %d calls", interval, stock.returned)
		}
	}
}

func TestSweeperRunDisabledDoesNotPanic(t *testing.T) {
	// time.NewTicker panics on a non-positive duration, so a missing guard
	// takes the whole API process down at startup rather than turning one
	// goroutine off. Pinned separately because the failure mode is a panic,
	// not a wrong result.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Run(0): panicked instead of disabling the sweeper: %v", r)
		}
	}()

	newSweeper(&fakeSweepStore{}, newFakeStock()).Run(context.Background(), 0)
}
