package cart

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/catalogue"
)

// Service-level failures, each mapping to exactly one API response.
var (
	// ErrProductNotFound means the id names nothing the shop sells.
	ErrProductNotFound = errors.New("cart: no such product")
	// ErrLineNotFound means the cart has no line for that product.
	ErrLineNotFound = errors.New("cart: line not found")
	// ErrInvalidQty means the quantity is outside MinQty..MaxQty.
	ErrInvalidQty = errors.New("cart: invalid quantity")
	// ErrInsufficientStock means the shop cannot supply that many units.
	ErrInsufficientStock = errors.New("cart: not enough stock")
)

// Store is the persistence surface the service needs.
type Store interface {
	Get(ctx context.Context, userID bson.ObjectID) (Cart, error)
	Save(ctx context.Context, cart Cart) error
}

// ProductSource resolves products by id. *catalogue.Repository satisfies it.
// The cart never stores prices, so this is consulted on every read.
type ProductSource interface {
	ProductsByID(ctx context.Context, ids []catalogue.ID) (map[catalogue.ID]catalogue.Product, error)
}

// Service holds the cart rules.
type Service struct {
	store    Store
	products ProductSource
	now      func() time.Time
}

// NewService builds the cart service.
func NewService(store Store, products ProductSource) *Service {
	return &Service{store: store, products: products, now: time.Now}
}

// View returns the shopper's cart, priced from the live catalogue.
func (s *Service) View(ctx context.Context, userID bson.ObjectID) (View, error) {
	cart, err := s.store.Get(ctx, userID)
	if err != nil {
		return View{}, err
	}
	return s.resolve(ctx, cart.Lines)
}

// AddItem adds a variant, incrementing the line if it is already present.
func (s *Service) AddItem(ctx context.Context, userID bson.ObjectID, productID catalogue.ID, qty int) (View, error) {
	if qty < MinQty || qty > MaxQty {
		return View{}, fmt.Errorf("%w: %d", ErrInvalidQty, qty)
	}

	// Resolve before touching the cart, so a bad request cannot create a line
	// pointing at something the shop does not sell.
	product, err := s.sellable(ctx, productID)
	if err != nil {
		return View{}, err
	}

	cart, err := s.store.Get(ctx, userID)
	if err != nil {
		return View{}, err
	}

	wanted := qty
	if i := findLine(cart.Lines, productID); i >= 0 {
		wanted = cart.Lines[i].Qty + qty
		if wanted > MaxQty {
			wanted = MaxQty
		}
	}
	// A sold-out item cannot be added at all, and an existing line cannot be
	// pushed past what remains on the shelf.
	if !product.CanFulfil(wanted) {
		return View{}, fmt.Errorf("%w: %s has %d left", ErrInsufficientStock, product.Name, product.Stock)
	}

	if i := findLine(cart.Lines, productID); i >= 0 {
		cart.Lines[i].Qty = wanted
	} else {
		cart.Lines = append(cart.Lines, StoredLine{ProductID: productID, Qty: qty})
	}
	return s.persist(ctx, cart)
}

// SetQty sets a line's quantity absolutely, not as a delta. Zero removes the
// line, which is how the frontend's stepper deletes.
func (s *Service) SetQty(ctx context.Context, userID bson.ObjectID, productID catalogue.ID, qty int) (View, error) {
	if qty < 0 || qty > MaxQty {
		return View{}, fmt.Errorf("%w: %d", ErrInvalidQty, qty)
	}

	cart, err := s.store.Get(ctx, userID)
	if err != nil {
		return View{}, err
	}

	i := findLine(cart.Lines, productID)
	if i < 0 {
		return View{}, fmt.Errorf("%w: %s", ErrLineNotFound, productID)
	}

	if qty == 0 {
		cart.Lines = append(cart.Lines[:i], cart.Lines[i+1:]...)
		return s.persist(ctx, cart)
	}

	// Raising a quantity is a purchase intent and must respect stock; the
	// product may have been retired since it was added.
	product, err := s.sellable(ctx, productID)
	if err != nil {
		return View{}, err
	}
	if !product.CanFulfil(qty) {
		return View{}, fmt.Errorf("%w: %s has %d left", ErrInsufficientStock, product.Name, product.Stock)
	}

	cart.Lines[i].Qty = qty
	return s.persist(ctx, cart)
}

// RemoveLine deletes a line outright. It does not consult the catalogue: a
// shopper must always be able to remove something, even a withdrawn variant.
func (s *Service) RemoveLine(ctx context.Context, userID bson.ObjectID, productID catalogue.ID) (View, error) {
	cart, err := s.store.Get(ctx, userID)
	if err != nil {
		return View{}, err
	}

	i := findLine(cart.Lines, productID)
	if i < 0 {
		return View{}, fmt.Errorf("%w: %s", ErrLineNotFound, productID)
	}
	cart.Lines = append(cart.Lines[:i], cart.Lines[i+1:]...)
	return s.persist(ctx, cart)
}

// Clear empties the cart, leaving the document in place.
func (s *Service) Clear(ctx context.Context, userID bson.ObjectID) (View, error) {
	return s.persist(ctx, Cart{UserID: userID, Lines: []StoredLine{}})
}

// Lines returns the resolved lines without saving, for order placement.
func (s *Service) Lines(ctx context.Context, userID bson.ObjectID) ([]Line, error) {
	view, err := s.View(ctx, userID)
	if err != nil {
		return nil, err
	}
	return view.Lines, nil
}

// sellable resolves one product that can currently be bought.
func (s *Service) sellable(ctx context.Context, id catalogue.ID) (catalogue.Product, error) {
	found, err := s.products.ProductsByID(ctx, []catalogue.ID{id})
	if err != nil {
		return catalogue.Product{}, err
	}
	product, ok := found[id]
	if !ok || !product.Active {
		return catalogue.Product{}, fmt.Errorf("%w: %s", ErrProductNotFound, id)
	}
	return product, nil
}

// persist saves the cart and returns the freshly priced view.
func (s *Service) persist(ctx context.Context, cart Cart) (View, error) {
	cart.UpdatedAt = s.now()
	if err := s.store.Save(ctx, cart); err != nil {
		return View{}, err
	}
	return s.resolve(ctx, cart.Lines)
}

// resolve joins stored lines with live catalogue data and totals them.
//
// A line whose product has been withdrawn is kept and flagged rather than
// dropped: silently removing something a shopper chose is worse than showing
// it as unavailable, and dropping it would also hide it from the checkout
// block. It contributes nothing to the totals.
func (s *Service) resolve(ctx context.Context, stored []StoredLine) (View, error) {
	ids := make([]catalogue.ID, 0, len(stored))
	for _, sl := range stored {
		ids = append(ids, sl.ProductID)
	}

	products, err := s.products.ProductsByID(ctx, ids)
	if err != nil {
		return View{}, err
	}

	lines := make([]Line, 0, len(stored))
	for _, sl := range stored {
		product, ok := products[sl.ProductID]
		if !ok || !product.Active {
			lines = append(lines, Line{ProductID: sl.ProductID, Qty: sl.Qty, Unavailable: true})
			continue
		}

		lines = append(lines, Line{
			ProductID: product.ID,
			Name:      product.Name,
			Form:      product.Form,
			Grad:      product.Grad,
			Stat2:     product.Stat2,
			UnitPrice: product.Price,
			UnitMRP:   product.MRP,
			Qty:       sl.Qty,
			LineTotal: product.Price * int64(sl.Qty),
			Stock:     product.Stock,
			SoldOut:   product.SoldOut(),
		})
	}

	totals := ComputeTotals(lines)
	view := View{
		Lines:        lines,
		Totals:       totals,
		FreeShipping: ComputeFreeShipping(totals.Subtotal),
		ItemCount:    ItemCount(lines),
	}
	for _, l := range lines {
		if l.Blocking() {
			view.HasBlockingIssues = true
			break
		}
	}
	return view, nil
}
