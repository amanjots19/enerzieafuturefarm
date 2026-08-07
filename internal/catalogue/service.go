package catalogue

import (
	"context"
	"errors"
	"fmt"
)

// ErrUnknownForm is returned when the caller asks to filter by a form the
// catalogue does not sell. It is a client mistake, not an empty result.
var ErrUnknownForm = errors.New("catalogue: unknown form")

// Reader is the read surface the service needs. *Repository satisfies it;
// tests substitute a stub.
type Reader interface {
	List(ctx context.Context, form Form, filtered bool) ([]Product, error)
	Get(ctx context.Context, id ID) (Product, error)
	Siblings(ctx context.Context, family Family, exclude ID) ([]Product, error)
	TrustTiles(ctx context.Context) ([]TrustTile, error)
}

// Service holds the catalogue's business rules.
type Service struct {
	repo Reader
}

// NewService builds a catalogue service over the given reader.
func NewService(repo Reader) *Service {
	return &Service{repo: repo}
}

// Detail is a product together with the other sizes in its family.
type Detail struct {
	Product  Product
	Siblings []Sibling
}

// Products returns the catalogue, narrowed by the raw `form` query value. An
// empty value or "all" returns everything; anything else the catalogue does
// not sell is ErrUnknownForm rather than an empty list, so a typo surfaces as
// a 400 instead of looking like a bug in the grid.
func (s *Service) Products(ctx context.Context, rawForm string) ([]Product, error) {
	form, filtered, ok := ParseFilter(rawForm)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownForm, rawForm)
	}

	products, err := s.repo.List(ctx, form, filtered)
	if err != nil {
		return nil, err
	}
	if products == nil {
		// Never nil: the response must serialise as [] rather than null.
		return []Product{}, nil
	}
	return products, nil
}

// Detail returns one product with its siblings. A retired product is reported
// as missing: it stays resolvable internally for old orders, but a shopper
// following a stale link should get a 404 rather than a page they cannot buy.
func (s *Service) Detail(ctx context.Context, id ID) (Detail, error) {
	product, err := s.repo.Get(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	if !product.Active {
		return Detail{}, fmt.Errorf("%w: %s", ErrProductNotFound, id)
	}

	others, err := s.repo.Siblings(ctx, product.Family, product.ID)
	if err != nil {
		return Detail{}, err
	}

	siblings := make([]Sibling, 0, len(others))
	for _, o := range others {
		siblings = append(siblings, o.AsSibling())
	}
	return Detail{Product: product, Siblings: siblings}, nil
}

// TrustTiles returns the statistics shown under the shop grid.
func (s *Service) TrustTiles(ctx context.Context) ([]TrustTile, error) {
	tiles, err := s.repo.TrustTiles(ctx)
	if err != nil {
		return nil, err
	}
	if tiles == nil {
		return []TrustTile{}, nil
	}
	return tiles, nil
}
