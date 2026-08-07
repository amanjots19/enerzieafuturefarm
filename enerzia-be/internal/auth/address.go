package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// minStreetLength is the shortest street line accepted. Six characters rules
// out "12" and "n/a" without rejecting a genuinely short Indian address.
const minStreetLength = 6

// emailPattern is deliberately the same loose check the frontend uses. A
// stricter regex here would reject addresses the UI accepted, which reads as
// the form lying to the shopper.
var emailPattern = regexp.MustCompile(`^\S+@\S+\.\S+$`)

// pinPattern is exactly six digits.
var pinPattern = regexp.MustCompile(`^\d{6}$`)

// ErrAddressNotFound is returned when an address id is not the caller's.
var ErrAddressNotFound = errors.New("auth: address not found")

// FieldProblem names the field that failed validation and the message to show.
type FieldProblem struct {
	Field   string
	Message string
}

// ValidateAddress checks a delivery address and returns the first problem.
//
// Order matters and is not arbitrary: it mirrors product.md §3.4 exactly, so
// the API surfaces the same message the shipped UI would have shown for the
// same input. Returning only the first keeps that parity — the frontend
// displays one message at a time.
func ValidateAddress(a Address) (FieldProblem, bool) {
	switch {
	case strings.TrimSpace(a.Name) == "":
		return FieldProblem{"name", "Please enter the name for delivery."}, false
	case !emailPattern.MatchString(a.Email):
		return FieldProblem{"email", "Please enter a valid email for order updates."}, false
	case len(strings.TrimSpace(a.Line1)) < minStreetLength:
		return FieldProblem{"line1", "Please enter your full street address."}, false
	case strings.TrimSpace(a.City) == "" || strings.TrimSpace(a.State) == "":
		return FieldProblem{"city", "Please enter your city and state."}, false
	case !pinPattern.MatchString(a.Pin):
		return FieldProblem{"pin", "PIN code must be 6 digits."}, false
	}
	return FieldProblem{}, true
}

// normaliseAddress trims fields a shopper is likely to paste with stray
// whitespace. Casing is left alone — names and street lines belong to the
// shopper.
func normaliseAddress(a Address) Address {
	return Address{
		ID:        a.ID,
		Label:     strings.TrimSpace(a.Label),
		Name:      strings.TrimSpace(a.Name),
		Email:     strings.TrimSpace(a.Email),
		Line1:     strings.TrimSpace(a.Line1),
		City:      strings.TrimSpace(a.City),
		State:     strings.TrimSpace(a.State),
		Pin:       strings.TrimSpace(a.Pin),
		IsDefault: a.IsDefault,
	}
}

/* ------------------------------------------------------------- repository */

// SetAddresses replaces a shopper's whole address list. The service computes
// the new list — including which entry is default — so the invariant "exactly
// one default" is applied in one write rather than several.
func (r *Repository) SetAddresses(ctx context.Context, userID bson.ObjectID, addrs []Address, now time.Time) error {
	if addrs == nil {
		addrs = []Address{}
	}

	res, err := r.users.UpdateByID(ctx, userID, bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "addresses", Value: addrs},
			{Key: "updatedAt", Value: now},
		}},
	})
	if err != nil {
		return fmt.Errorf("auth: set addresses: %w", err)
	}
	// No upsert: an address must attach to a real account, never conjure one.
	if res.MatchedCount == 0 {
		return ErrUserNotFound
	}
	return nil
}

/* ---------------------------------------------------------------- service */

// Addresses returns the shopper's saved addresses, default first. A shopper
// with none gets an empty slice, not an error — a blank address book is a
// state, not a failure.
func (s *Service) Addresses(ctx context.Context, userID bson.ObjectID) ([]Address, error) {
	user, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return sortAddresses(user.Addresses), nil
}

// AddAddress saves a new address and returns it.
//
// The first address a shopper saves becomes their default automatically, so
// there is never a state where addresses exist but none is selected.
func (s *Service) AddAddress(ctx context.Context, userID bson.ObjectID, addr Address) (Address, FieldProblem, error) {
	addr = normaliseAddress(addr)
	if problem, ok := ValidateAddress(addr); !ok {
		return Address{}, problem, nil
	}

	user, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return Address{}, FieldProblem{}, err
	}

	addr.ID = bson.NewObjectID()
	if len(user.Addresses) == 0 {
		addr.IsDefault = true
	}

	next := append(append([]Address{}, user.Addresses...), addr)
	next = applyDefault(next, addr.ID, addr.IsDefault)

	if err := s.store.SetAddresses(ctx, userID, next, s.now()); err != nil {
		return Address{}, FieldProblem{}, err
	}
	return findAddress(next, addr.ID), FieldProblem{}, nil
}

// UpdateAddress replaces one address wholesale.
func (s *Service) UpdateAddress(
	ctx context.Context, userID, addressID bson.ObjectID, addr Address,
) (Address, FieldProblem, error) {
	addr = normaliseAddress(addr)
	if problem, ok := ValidateAddress(addr); !ok {
		return Address{}, problem, nil
	}

	user, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return Address{}, FieldProblem{}, err
	}

	i := indexOfAddress(user.Addresses, addressID)
	if i < 0 {
		return Address{}, FieldProblem{}, fmt.Errorf("%w: %s", ErrAddressNotFound, addressID.Hex())
	}

	next := append([]Address{}, user.Addresses...)
	// Clearing the flag on the only default is ignored: a shopper with
	// addresses always has one selected.
	wasDefault := next[i].IsDefault
	addr.ID = addressID
	next[i] = addr
	next = applyDefault(next, addressID, addr.IsDefault || wasDefault)

	if err := s.store.SetAddresses(ctx, userID, next, s.now()); err != nil {
		return Address{}, FieldProblem{}, err
	}
	return findAddress(next, addressID), FieldProblem{}, nil
}

// DeleteAddress removes an address and returns what is left. Deleting the
// default promotes the next one, so the invariant survives.
//
// A placed order is unaffected: orders carry a copy of the address, not a
// reference (schema.md decision 3).
func (s *Service) DeleteAddress(ctx context.Context, userID, addressID bson.ObjectID) ([]Address, error) {
	user, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	i := indexOfAddress(user.Addresses, addressID)
	if i < 0 {
		return nil, fmt.Errorf("%w: %s", ErrAddressNotFound, addressID.Hex())
	}

	next := append([]Address{}, user.Addresses[:i]...)
	next = append(next, user.Addresses[i+1:]...)

	if len(next) > 0 && !hasDefault(next) {
		next[0].IsDefault = true
	}

	if err := s.store.SetAddresses(ctx, userID, next, s.now()); err != nil {
		return nil, err
	}
	return sortAddresses(next), nil
}

// AddressFor resolves the address an order should ship to: the one named, or
// the default when none is named.
func (s *Service) AddressFor(ctx context.Context, userID bson.ObjectID, addressID *bson.ObjectID) (Address, error) {
	user, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return Address{}, err
	}
	if len(user.Addresses) == 0 {
		return Address{}, ErrAddressNotFound
	}

	if addressID == nil {
		for _, a := range user.Addresses {
			if a.IsDefault {
				return a, nil
			}
		}
		return user.Addresses[0], nil
	}

	i := indexOfAddress(user.Addresses, *addressID)
	if i < 0 {
		return Address{}, fmt.Errorf("%w: %s", ErrAddressNotFound, addressID.Hex())
	}
	return user.Addresses[i], nil
}

/* ----------------------------------------------------------------- helpers */

// applyDefault enforces "exactly one default". MongoDB cannot express the
// constraint, so it is applied here in the same write.
func applyDefault(addrs []Address, id bson.ObjectID, makeDefault bool) []Address {
	if makeDefault {
		for i := range addrs {
			addrs[i].IsDefault = addrs[i].ID == id
		}
		return addrs
	}
	if !hasDefault(addrs) && len(addrs) > 0 {
		addrs[0].IsDefault = true
	}
	return addrs
}

func hasDefault(addrs []Address) bool {
	for _, a := range addrs {
		if a.IsDefault {
			return true
		}
	}
	return false
}

func indexOfAddress(addrs []Address, id bson.ObjectID) int {
	for i, a := range addrs {
		if a.ID == id {
			return i
		}
	}
	return -1
}

func findAddress(addrs []Address, id bson.ObjectID) Address {
	if i := indexOfAddress(addrs, id); i >= 0 {
		return addrs[i]
	}
	return Address{}
}

// sortAddresses puts the default first, leaving the rest in insertion order,
// so the checkout form can select addrs[0] without extra logic.
func sortAddresses(addrs []Address) []Address {
	out := make([]Address, 0, len(addrs))
	for _, a := range addrs {
		if a.IsDefault {
			out = append(out, a)
		}
	}
	for _, a := range addrs {
		if !a.IsDefault {
			out = append(out, a)
		}
	}
	return out
}
