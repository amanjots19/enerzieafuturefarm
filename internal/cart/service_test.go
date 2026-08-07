package cart_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/cart"
	"github.com/enerzia/enerzia-be/internal/catalogue"
)

// memStore is an in-memory cart.Store.
type memStore struct {
	carts   map[bson.ObjectID]cart.Cart
	getErr  error
	saveErr error
	saves   int
}

func newMemStore() *memStore {
	return &memStore{carts: map[bson.ObjectID]cart.Cart{}}
}

func (m *memStore) Get(_ context.Context, userID bson.ObjectID) (cart.Cart, error) {
	if m.getErr != nil {
		return cart.Cart{}, m.getErr
	}
	c, ok := m.carts[userID]
	if !ok {
		return cart.Cart{UserID: userID}, nil
	}
	// Copy the slice so a caller mutating it cannot reach into the store.
	c.Lines = append([]cart.StoredLine(nil), c.Lines...)
	return c, nil
}

func (m *memStore) Save(_ context.Context, c cart.Cart) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saves++
	m.carts[c.UserID] = c
	return nil
}

// stubProducts serves the real seeded catalogue unless told otherwise.
type stubProducts struct {
	products []catalogue.Product
	err      error
	calls    int
}

func newStubProducts() *stubProducts {
	return &stubProducts{products: catalogue.SeedProducts()}
}

func (s *stubProducts) ProductsByID(_ context.Context, ids []catalogue.ID) (map[catalogue.ID]catalogue.Product, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	wanted := make(map[catalogue.ID]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	out := make(map[catalogue.ID]catalogue.Product)
	for _, p := range s.products {
		if wanted[p.ID] {
			out[p.ID] = p
		}
	}
	return out, nil
}

// mutate applies a change to one seeded product.
func (s *stubProducts) mutate(t *testing.T, id catalogue.ID, f func(*catalogue.Product)) {
	t.Helper()
	for i := range s.products {
		if s.products[i].ID == id {
			f(&s.products[i])
			return
		}
	}
	t.Fatalf("no seeded product %s", id)
}

func (s *stubProducts) setStock(t *testing.T, id catalogue.ID, stock int) {
	t.Helper()
	s.mutate(t, id, func(p *catalogue.Product) { p.Stock = stock })
}

func newSvc(t *testing.T) (*cart.Service, *memStore, *stubProducts) {
	t.Helper()
	store, products := newMemStore(), newStubProducts()
	return cart.NewService(store, products), store, products
}

var testUser = bson.NewObjectID()

/* --------------------------------------------------------------------- View */

func TestViewOfANeverUsedCartIsEmptyNotAnError(t *testing.T) {
	svc, _, _ := newSvc(t)

	view, err := svc.View(t.Context(), testUser)
	if err != nil {
		t.Fatalf("View() error = %v, want nil", err)
	}
	if len(view.Lines) != 0 || view.ItemCount != 0 || view.Totals != (cart.Totals{}) {
		t.Errorf("empty cart should be empty and total zero, got %+v", view)
	}
	if view.FreeShipping.Qualified {
		t.Error("an empty cart must not report free delivery as earned")
	}
	if view.HasBlockingIssues {
		t.Error("an empty cart cannot block checkout")
	}
}

func TestViewPricesFromTheLiveCatalogue(t *testing.T) {
	svc, store, _ := newSvc(t)
	const id = catalogue.ID("tablets-120")
	store.carts[testUser] = cart.Cart{UserID: testUser, Lines: []cart.StoredLine{{ProductID: id, Qty: 3}}}

	view, err := svc.View(t.Context(), testUser)
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	if len(view.Lines) != 1 {
		t.Fatalf("%d lines, want 1", len(view.Lines))
	}

	l := view.Lines[0]
	if l.ProductID != id {
		t.Errorf("variantId = %v, want %v", l.ProductID, id)
	}
	if l.UnitPrice != 38000 || l.UnitMRP != 47000 || l.LineTotal != 114000 {
		t.Errorf("pricing = %d/%d/%d, want 38000/47000/114000", l.UnitPrice, l.UnitMRP, l.LineTotal)
	}
	// The whole product is joined, name and all.
	if l.Name != "Spirulina Tablets 500 mg — 120 tabs" || l.Stat2 != "30 days" {
		t.Errorf("catalogue data not joined: %+v", l)
	}
	if l.Stock <= 0 || l.SoldOut {
		t.Errorf("stock = %d soldOut = %v, want stock exposed", l.Stock, l.SoldOut)
	}
}

func TestViewReflectsACataloguePriceChange(t *testing.T) {
	// The stored cart holds no prices, so a repricing shows up immediately.
	svc, store, products := newSvc(t)
	const id = catalogue.ID("tablets-120")
	store.carts[testUser] = cart.Cart{UserID: testUser, Lines: []cart.StoredLine{{ProductID: id, Qty: 1}}}

	products.mutate(t, id, func(p *catalogue.Product) { p.Price = 30000 })

	after, err := svc.View(t.Context(), testUser)
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	if after.Totals.Subtotal != 30000 {
		t.Errorf("subtotal = %d, want the new price 30000", after.Totals.Subtotal)
	}
}

func TestViewSurvivesAProductRename(t *testing.T) {
	// The whole point of a stable id: renaming must not orphan the line.
	svc, store, products := newSvc(t)
	const id = catalogue.ID("tablets-120")
	store.carts[testUser] = cart.Cart{UserID: testUser, Lines: []cart.StoredLine{{ProductID: id, Qty: 1}}}

	products.mutate(t, id, func(p *catalogue.Product) { p.Name = "Spirulina Tablets — 120 count" })

	view, err := svc.View(t.Context(), testUser)
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	if len(view.Lines) != 1 {
		t.Fatalf("%d lines, want the line to survive the rename", len(view.Lines))
	}
	if view.Lines[0].Name != "Spirulina Tablets — 120 count" {
		t.Errorf("name = %q, want the new one", view.Lines[0].Name)
	}
}

func TestViewFlagsAWithdrawnProductRatherThanDroppingIt(t *testing.T) {
	// Silently removing a shopper's choice hides the problem; flagging it
	// explains the blocked checkout.
	svc, store, _ := newSvc(t)
	const good = catalogue.ID("tablets-120")
	const gone = catalogue.ID("ghost-1kg")

	store.carts[testUser] = cart.Cart{UserID: testUser, Lines: []cart.StoredLine{
		{ProductID: good, Qty: 1},
		{ProductID: gone, Qty: 1},
	}}

	view, err := svc.View(t.Context(), testUser)
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	if len(view.Lines) != 2 {
		t.Fatalf("%d lines, want both kept", len(view.Lines))
	}
	if !view.Lines[1].Unavailable {
		t.Error("the withdrawn line is not flagged")
	}
	// It contributes nothing to the money.
	if view.Totals.Subtotal != 38000 {
		t.Errorf("subtotal = %d, want only the sellable line", view.Totals.Subtotal)
	}
	if !view.HasBlockingIssues {
		t.Error("a withdrawn line must block checkout")
	}
}

func TestViewFlagsALineThatSoldOutAfterAdding(t *testing.T) {
	svc, store, products := newSvc(t)
	const id = catalogue.ID("tablets-120")
	store.carts[testUser] = cart.Cart{UserID: testUser, Lines: []cart.StoredLine{{ProductID: id, Qty: 2}}}

	products.setStock(t, "tablets-120", 0)

	view, err := svc.View(t.Context(), testUser)
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	if !view.Lines[0].SoldOut || !view.HasBlockingIssues {
		t.Errorf("line = %+v, want it flagged sold out and blocking", view.Lines[0])
	}
}

func TestViewFlagsALineWantingMoreThanRemains(t *testing.T) {
	svc, store, products := newSvc(t)
	const id = catalogue.ID("tablets-120")
	store.carts[testUser] = cart.Cart{UserID: testUser, Lines: []cart.StoredLine{{ProductID: id, Qty: 5}}}

	products.setStock(t, "tablets-120", 2)

	view, err := svc.View(t.Context(), testUser)
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	if !view.HasBlockingIssues {
		t.Error("wanting 5 of 2 must block checkout")
	}
}

func TestViewSurfacesFailures(t *testing.T) {
	t.Run("store", func(t *testing.T) {
		svc, store, _ := newSvc(t)
		store.getErr = errors.New("boom")
		if _, err := svc.View(t.Context(), testUser); err == nil {
			t.Error("View() error = nil, want the store failure")
		}
	})
	t.Run("catalogue", func(t *testing.T) {
		svc, store, products := newSvc(t)
		store.carts[testUser] = cart.Cart{UserID: testUser,
			Lines: []cart.StoredLine{{ProductID: "ghost-1kg", Qty: 1}}}
		products.err = errors.New("boom")
		if _, err := svc.View(t.Context(), testUser); err == nil {
			t.Error("View() error = nil, want the catalogue failure")
		}
	})
}

/* ------------------------------------------------------------------ AddItem */

func TestAddItemCreatesALine(t *testing.T) {
	svc, store, _ := newSvc(t)
	const id = catalogue.ID("powder-100g")

	view, err := svc.AddItem(t.Context(), testUser, id, 1)
	if err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}
	if len(view.Lines) != 1 || view.ItemCount != 1 {
		t.Fatalf("view = %+v, want one line of one", view)
	}
	// Only the variant id and the count are persisted.
	stored := store.carts[testUser].Lines
	if len(stored) != 1 || stored[0].ProductID != id || stored[0].Qty != 1 {
		t.Errorf("stored = %+v", stored)
	}
}

func TestAddItemIncrementsAnExistingLine(t *testing.T) {
	svc, store, _ := newSvc(t)
	const id = catalogue.ID("powder-100g")

	if _, err := svc.AddItem(t.Context(), testUser, id, 1); err != nil {
		t.Fatalf("first AddItem() error = %v", err)
	}
	view, err := svc.AddItem(t.Context(), testUser, id, 2)
	if err != nil {
		t.Fatalf("second AddItem() error = %v", err)
	}
	if len(view.Lines) != 1 || view.Lines[0].Qty != 3 {
		t.Errorf("lines = %+v, want one line of three", view.Lines)
	}
	if got := len(store.carts[testUser].Lines); got != 1 {
		t.Errorf("stored %d lines, want 1", got)
	}
}

func TestAddItemKeepsVariantsSeparate(t *testing.T) {
	svc, _, _ := newSvc(t)

	if _, err := svc.AddItem(t.Context(), testUser, "powder-100g", 1); err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}
	view, err := svc.AddItem(t.Context(), testUser, "powder-250g", 1)
	if err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}
	if len(view.Lines) != 2 {
		t.Errorf("%d lines, want 2 — different sizes are different lines", len(view.Lines))
	}
}

func TestAddItemRejectsBadQuantities(t *testing.T) {
	svc, store, _ := newSvc(t)
	const id = catalogue.ID("powder-100g")

	for _, qty := range []int{0, -1, cart.MaxQty + 1} {
		if _, err := svc.AddItem(t.Context(), testUser, id, qty); !errors.Is(err, cart.ErrInvalidQty) {
			t.Errorf("AddItem(qty=%d) error = %v, want ErrInvalidQty", qty, err)
		}
	}
	if store.saves != 0 {
		t.Error("an invalid quantity must not write to the store")
	}
}

func TestAddItemRejectsAnUnknownProduct(t *testing.T) {
	svc, store, _ := newSvc(t)

	_, err := svc.AddItem(t.Context(), testUser, "ghost-1kg", 1)
	if !errors.Is(err, cart.ErrProductNotFound) {
		t.Errorf("AddItem() error = %v, want ErrVariantNotFound", err)
	}
	if store.saves != 0 {
		t.Error("an unsellable line must never be written")
	}
}

func TestAddItemRejectsARetiredProduct(t *testing.T) {
	svc, _, products := newSvc(t)
	const id = catalogue.ID("powder-100g")
	products.mutate(t, id, func(p *catalogue.Product) { p.Active = false })

	if _, err := svc.AddItem(t.Context(), testUser, id, 1); !errors.Is(err, cart.ErrProductNotFound) {
		t.Errorf("AddItem() error = %v, want a retired product to be unsellable", err)
	}
}

func TestAddItemRejectsASoldOutProduct(t *testing.T) {
	svc, store, products := newSvc(t)
	products.setStock(t, "powder-100g", 0)

	_, err := svc.AddItem(t.Context(), testUser, "powder-100g", 1)
	if !errors.Is(err, cart.ErrInsufficientStock) {
		t.Fatalf("AddItem() error = %v, want ErrInsufficientStock", err)
	}
	if store.saves != 0 {
		t.Error("a sold-out add must not write to the store")
	}
}

func TestAddItemRefusesToExceedStock(t *testing.T) {
	svc, _, products := newSvc(t)
	const id = catalogue.ID("powder-100g")
	products.setStock(t, "powder-100g", 2)

	if _, err := svc.AddItem(t.Context(), testUser, id, 2); err != nil {
		t.Fatalf("AddItem(2 of 2) error = %v, want nil", err)
	}
	// The increment would take it to 3, past what remains.
	if _, err := svc.AddItem(t.Context(), testUser, id, 1); !errors.Is(err, cart.ErrInsufficientStock) {
		t.Errorf("AddItem() error = %v, want ErrInsufficientStock", err)
	}
}

func TestAddItemSurfacesFailures(t *testing.T) {
	t.Run("catalogue", func(t *testing.T) {
		svc, _, products := newSvc(t)
		products.err = errors.New("boom")
		if _, err := svc.AddItem(t.Context(), testUser, "ghost-1kg", 1); err == nil {
			t.Error("AddItem() error = nil, want the catalogue failure")
		}
	})
	t.Run("store get", func(t *testing.T) {
		svc, store, _ := newSvc(t)
		store.getErr = errors.New("boom")
		if _, err := svc.AddItem(t.Context(), testUser, "powder-100g", 1); err == nil {
			t.Error("AddItem() error = nil, want the store failure")
		}
	})
	t.Run("store save", func(t *testing.T) {
		svc, store, _ := newSvc(t)
		store.saveErr = errors.New("boom")
		if _, err := svc.AddItem(t.Context(), testUser, "powder-100g", 1); err == nil {
			t.Error("AddItem() error = nil, want the store failure")
		}
	})
}

/* ------------------------------------------------------------------- SetQty */

func seedCart(t *testing.T, svc *cart.Service, _ *stubProducts) catalogue.ID {
	t.Helper()
	const id = catalogue.ID("tablets-120")
	if _, err := svc.AddItem(t.Context(), testUser, id, 2); err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}
	return id
}

func TestSetQtyIsAbsoluteNotADelta(t *testing.T) {
	svc, _, products := newSvc(t)
	id := seedCart(t, svc, products)

	view, err := svc.SetQty(t.Context(), testUser, id, 5)
	if err != nil {
		t.Fatalf("SetQty() error = %v", err)
	}
	if view.Lines[0].Qty != 5 {
		t.Errorf("qty = %d, want 5 (absolute, not 2+5)", view.Lines[0].Qty)
	}
}

func TestSetQtyZeroDeletesTheLine(t *testing.T) {
	svc, store, products := newSvc(t)
	id := seedCart(t, svc, products)

	view, err := svc.SetQty(t.Context(), testUser, id, 0)
	if err != nil {
		t.Fatalf("SetQty() error = %v", err)
	}
	if len(view.Lines) != 0 || len(store.carts[testUser].Lines) != 0 {
		t.Error("the line was not removed")
	}
}

func TestSetQtyZeroWorksForAWithdrawnVariant(t *testing.T) {
	// A shopper must always be able to get rid of something, even if the shop
	// no longer sells it.
	svc, store, _ := newSvc(t)
	const gone = catalogue.ID("ghost-1kg")
	store.carts[testUser] = cart.Cart{UserID: testUser, Lines: []cart.StoredLine{{ProductID: gone, Qty: 1}}}

	view, err := svc.SetQty(t.Context(), testUser, gone, 0)
	if err != nil {
		t.Fatalf("SetQty(0) error = %v, want removal to work regardless", err)
	}
	if len(view.Lines) != 0 {
		t.Error("the withdrawn line was not removed")
	}
}

func TestSetQtyRejectsBadValues(t *testing.T) {
	svc, _, products := newSvc(t)
	id := seedCart(t, svc, products)

	for _, qty := range []int{-1, cart.MaxQty + 1} {
		if _, err := svc.SetQty(t.Context(), testUser, id, qty); !errors.Is(err, cart.ErrInvalidQty) {
			t.Errorf("SetQty(%d) error = %v, want ErrInvalidQty", qty, err)
		}
	}
}

func TestSetQtyRefusesToExceedStock(t *testing.T) {
	svc, _, products := newSvc(t)
	id := seedCart(t, svc, products)
	products.setStock(t, "tablets-120", 3)

	if _, err := svc.SetQty(t.Context(), testUser, id, 4); !errors.Is(err, cart.ErrInsufficientStock) {
		t.Errorf("SetQty() error = %v, want ErrInsufficientStock", err)
	}
}

func TestSetQtyRejectsAnUnknownLine(t *testing.T) {
	svc, _, products := newSvc(t)
	seedCart(t, svc, products)

	if _, err := svc.SetQty(t.Context(), testUser, "ghost-1kg", 2); !errors.Is(err, cart.ErrLineNotFound) {
		t.Errorf("SetQty() error = %v, want ErrLineNotFound", err)
	}
}

func TestSetQtySurfacesFailures(t *testing.T) {
	svc, store, products := newSvc(t)
	id := seedCart(t, svc, products)

	store.getErr = errors.New("boom")
	if _, err := svc.SetQty(t.Context(), testUser, id, 3); err == nil {
		t.Error("SetQty() error = nil, want the store failure")
	}
	store.getErr = nil

	products.err = errors.New("boom")
	if _, err := svc.SetQty(t.Context(), testUser, id, 3); err == nil {
		t.Error("SetQty() error = nil, want the catalogue failure")
	}
}

/* --------------------------------------------------------------- RemoveLine */

func TestRemoveLine(t *testing.T) {
	svc, store, products := newSvc(t)
	id := seedCart(t, svc, products)

	view, err := svc.RemoveLine(t.Context(), testUser, id)
	if err != nil {
		t.Fatalf("RemoveLine() error = %v", err)
	}
	if len(view.Lines) != 0 || len(store.carts[testUser].Lines) != 0 {
		t.Error("the line was not removed")
	}
}

func TestRemoveLineRejectsAnUnknownLine(t *testing.T) {
	svc, _, _ := newSvc(t)

	if _, err := svc.RemoveLine(t.Context(), testUser, "ghost-1kg"); !errors.Is(err, cart.ErrLineNotFound) {
		t.Errorf("RemoveLine() error = %v, want ErrLineNotFound", err)
	}
}

func TestRemoveLineKeepsTheOtherLines(t *testing.T) {
	svc, _, _ := newSvc(t)
	const powder = catalogue.ID("powder-100g")
	const tablets = catalogue.ID("tablets-120")

	if _, err := svc.AddItem(t.Context(), testUser, powder, 1); err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}
	if _, err := svc.AddItem(t.Context(), testUser, tablets, 1); err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}

	view, err := svc.RemoveLine(t.Context(), testUser, powder)
	if err != nil {
		t.Fatalf("RemoveLine() error = %v", err)
	}
	if len(view.Lines) != 1 || view.Lines[0].ProductID != tablets {
		t.Errorf("remaining = %+v, want only tablets", view.Lines)
	}
}

func TestRemoveLineSurfacesStoreFailures(t *testing.T) {
	svc, store, products := newSvc(t)
	id := seedCart(t, svc, products)

	store.getErr = errors.New("boom")
	if _, err := svc.RemoveLine(t.Context(), testUser, id); err == nil {
		t.Error("RemoveLine() error = nil, want the store failure")
	}
}

/* -------------------------------------------------------------------- Clear */

func TestClearEmptiesTheCart(t *testing.T) {
	svc, store, products := newSvc(t)
	seedCart(t, svc, products)

	view, err := svc.Clear(t.Context(), testUser)
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if len(view.Lines) != 0 || view.ItemCount != 0 {
		t.Errorf("view = %+v, want empty", view)
	}
	if len(store.carts[testUser].Lines) != 0 {
		t.Error("the stored cart still has lines")
	}
}

func TestClearOnAnAlreadyEmptyCartIsFine(t *testing.T) {
	svc, _, _ := newSvc(t)
	if _, err := svc.Clear(t.Context(), testUser); err != nil {
		t.Errorf("Clear() error = %v, want nil", err)
	}
}

func TestClearSurfacesFailures(t *testing.T) {
	svc, store, _ := newSvc(t)
	store.saveErr = errors.New("boom")

	if _, err := svc.Clear(t.Context(), testUser); err == nil {
		t.Error("Clear() error = nil, want the store failure")
	}
}

/* ------------------------------------------------------------------- Lines */

func TestLinesReturnsTheResolvedLines(t *testing.T) {
	svc, _, products := newSvc(t)
	seedCart(t, svc, products)

	lines, err := svc.Lines(t.Context(), testUser)
	if err != nil {
		t.Fatalf("Lines() error = %v", err)
	}
	if len(lines) != 1 || lines[0].Qty != 2 {
		t.Errorf("lines = %+v, want the seeded line", lines)
	}
}

func TestLinesSurfacesFailures(t *testing.T) {
	svc, store, _ := newSvc(t)
	store.getErr = errors.New("boom")

	if _, err := svc.Lines(t.Context(), testUser); err == nil {
		t.Error("Lines() error = nil, want the store failure")
	}
}

/* ---------------------------------------------------------------- isolation */

func TestCartsAreIsolatedPerShopper(t *testing.T) {
	svc, _, _ := newSvc(t)
	other := bson.NewObjectID()

	if _, err := svc.AddItem(t.Context(), testUser, "powder-100g", 2); err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}

	view, err := svc.View(t.Context(), other)
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
	if len(view.Lines) != 0 {
		t.Error("one shopper can see another's cart")
	}
}

func TestUpdatedAtIsStamped(t *testing.T) {
	svc, store, _ := newSvc(t)
	before := time.Now().Add(-time.Second)

	if _, err := svc.AddItem(t.Context(), testUser, "powder-100g", 1); err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}
	if !store.carts[testUser].UpdatedAt.After(before) {
		t.Error("UpdatedAt was not stamped on write")
	}
}

func TestProductsAreFetchedInOneQueryPerRead(t *testing.T) {
	// Three lines must not mean three round trips.
	svc, store, products := newSvc(t)
	store.carts[testUser] = cart.Cart{UserID: testUser, Lines: []cart.StoredLine{
		{ProductID: "powder-100g", Qty: 1},
		{ProductID: "tablets-120", Qty: 1},
		{ProductID: "refill-250g", Qty: 1},
	}}

	products.calls = 0
	if _, err := svc.View(t.Context(), testUser); err != nil {
		t.Fatalf("View() error = %v", err)
	}
	if products.calls != 1 {
		t.Errorf("products queried %d times for 3 lines, want 1", products.calls)
	}
}
