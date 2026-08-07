package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/auth"
)

// memStore is an in-memory auth.Store, so sign-in rules are exercised without
// a database in the way.
type memStore struct {
	codes []auth.OTPCode
	users map[string]auth.User

	createErr     error
	countErr      error
	latestErr     error
	attemptErr    error
	consumeErr    error
	upsertErr     error
	userByIDErr   error
	setAddressErr error

	consumeReturns *bool // override the "was it consumed" answer
	attemptsLogged int
}

func newMemStore() *memStore {
	return &memStore{users: map[string]auth.User{}}
}

func (m *memStore) CreateCode(_ context.Context, code auth.OTPCode) error {
	if m.createErr != nil {
		return m.createErr
	}
	if code.ID.IsZero() {
		code.ID = bson.NewObjectID()
	}
	m.codes = append(m.codes, code)
	return nil
}

func (m *memStore) LatestCode(_ context.Context, phone string) (auth.OTPCode, error) {
	if m.latestErr != nil {
		return auth.OTPCode{}, m.latestErr
	}
	var newest auth.OTPCode
	var found bool
	for _, c := range m.codes {
		if c.Phone == phone && (!found || c.CreatedAt.After(newest.CreatedAt)) {
			newest, found = c, true
		}
	}
	if !found {
		return auth.OTPCode{}, auth.ErrCodeNotFound
	}
	return newest, nil
}

func (m *memStore) CountCodesSince(_ context.Context, phone string, since time.Time) (int64, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	var n int64
	for _, c := range m.codes {
		if c.Phone == phone && !c.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (m *memStore) RecordFailedAttempt(_ context.Context, id bson.ObjectID) error {
	if m.attemptErr != nil {
		return m.attemptErr
	}
	m.attemptsLogged++
	for i := range m.codes {
		if m.codes[i].ID == id {
			m.codes[i].Attempts++
		}
	}
	return nil
}

func (m *memStore) ConsumeCode(_ context.Context, id bson.ObjectID) (bool, error) {
	if m.consumeErr != nil {
		return false, m.consumeErr
	}
	if m.consumeReturns != nil {
		return *m.consumeReturns, nil
	}
	for i := range m.codes {
		if m.codes[i].ID == id && !m.codes[i].Consumed {
			m.codes[i].Consumed = true
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) UpsertUser(_ context.Context, phone string, now time.Time) (auth.User, error) {
	if m.upsertErr != nil {
		return auth.User{}, m.upsertErr
	}
	if u, ok := m.users[phone]; ok {
		u.UpdatedAt = now
		m.users[phone] = u
		return u, nil
	}
	u := auth.User{ID: bson.NewObjectID(), Phone: phone, CreatedAt: now, UpdatedAt: now}
	m.users[phone] = u
	return u, nil
}

func (m *memStore) SetAddresses(_ context.Context, userID bson.ObjectID, addrs []auth.Address, now time.Time) error {
	if m.setAddressErr != nil {
		return m.setAddressErr
	}
	for phone, u := range m.users {
		if u.ID == userID {
			u.Addresses = append([]auth.Address(nil), addrs...)
			u.UpdatedAt = now
			m.users[phone] = u
			return nil
		}
	}
	return auth.ErrUserNotFound
}

func (m *memStore) UserByID(_ context.Context, id bson.ObjectID) (auth.User, error) {
	if m.userByIDErr != nil {
		return auth.User{}, m.userByIDErr
	}
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return auth.User{}, auth.ErrUserNotFound
}

// recordingSender captures what would have been delivered.
type recordingSender struct {
	phone, code string
	sends       int
	err         error
}

func (s *recordingSender) Send(_ context.Context, phone, code string) error {
	if s.err != nil {
		return s.err
	}
	s.phone, s.code = phone, code
	s.sends++
	return nil
}

func newService(t *testing.T, store auth.Store, sender auth.Sender, reveal bool) *auth.Service {
	t.Helper()
	return auth.NewService(auth.ServiceConfig{
		Store:      store,
		Sender:     sender,
		Tokens:     auth.NewTokenIssuer(tokenSecret, auth.TokenTTL),
		Pepper:     pepper,
		RevealCode: reveal,
	})
}

const testPhone = "9876543210"

/* -------------------------------------------------------------- RequestCode */

func TestRequestCodeStoresAHashAndSendsThePlaintext(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)

	result, err := svc.RequestCode(t.Context(), testPhone)
	if err != nil {
		t.Fatalf("RequestCode() error = %v, want nil", err)
	}
	if result.ExpiresIn != auth.CodeTTL || result.ResendAfter != auth.ResendInterval {
		t.Errorf("result = %+v, want the roadmap's TTL and resend interval", result)
	}
	if sender.sends != 1 || sender.phone != testPhone {
		t.Fatalf("sender got %d sends for %q", sender.sends, sender.phone)
	}
	if len(store.codes) != 1 {
		t.Fatalf("stored %d codes, want 1", len(store.codes))
	}

	stored := store.codes[0]
	if stored.CodeHash == sender.code {
		t.Error("the plaintext code was stored; it must be hashed")
	}
	if !auth.CodeMatches(pepper, testPhone, sender.code, stored.CodeHash) {
		t.Error("the stored hash does not match the code that was sent")
	}
	if stored.Consumed || stored.Attempts != 0 {
		t.Errorf("a fresh code should be unspent with no attempts, got %+v", stored)
	}
}

func TestRequestCodeHidesTheCodeInProduction(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}

	if result, err := newService(t, store, sender, false).RequestCode(t.Context(), testPhone); err != nil {
		t.Fatalf("RequestCode() error = %v", err)
	} else if result.DevCode != "" {
		t.Errorf("DevCode = %q, want it empty when RevealCode is false", result.DevCode)
	}
}

func TestRequestCodeRevealsTheCodeInDevelopment(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}

	result, err := newService(t, store, sender, true).RequestCode(t.Context(), testPhone)
	if err != nil {
		t.Fatalf("RequestCode() error = %v", err)
	}
	if result.DevCode != sender.code {
		t.Errorf("DevCode = %q, want the code that was sent (%q)", result.DevCode, sender.code)
	}
}

func TestRequestCodeRejectsABadPhoneNumber(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)

	for _, phone := range []string{"", "12345", "+919876543210", "98765abcde"} {
		t.Run(phone, func(t *testing.T) {
			if _, err := svc.RequestCode(t.Context(), phone); !errors.Is(err, auth.ErrInvalidPhone) {
				t.Errorf("RequestCode(%q) error = %v, want ErrInvalidPhone", phone, err)
			}
		})
	}
	if sender.sends != 0 {
		t.Error("an invalid number must not reach the sender")
	}
}

func TestRequestCodeEnforcesThePerWindowCap(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)

	// Pre-load the window with the maximum, all older than the resend gap.
	old := time.Now().Add(-5 * time.Minute)
	for range auth.MaxRequestsPerWindow {
		store.codes = append(store.codes, auth.OTPCode{
			ID: bson.NewObjectID(), Phone: testPhone, CreatedAt: old,
		})
	}

	if _, err := svc.RequestCode(t.Context(), testPhone); !errors.Is(err, auth.ErrRateLimited) {
		t.Fatalf("RequestCode() error = %v, want ErrRateLimited", err)
	}
	if sender.sends != 0 {
		t.Error("a rate-limited request must not cost a message")
	}
}

func TestRequestCodeEnforcesTheResendInterval(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)

	if _, err := svc.RequestCode(t.Context(), testPhone); err != nil {
		t.Fatalf("first RequestCode() error = %v", err)
	}
	// Immediately again: under the cap, but inside the resend gap.
	if _, err := svc.RequestCode(t.Context(), testPhone); !errors.Is(err, auth.ErrRateLimited) {
		t.Errorf("second RequestCode() error = %v, want ErrRateLimited", err)
	}
}

func TestRequestCodeAllowsAResendAfterTheInterval(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)

	store.codes = append(store.codes, auth.OTPCode{
		ID: bson.NewObjectID(), Phone: testPhone,
		CreatedAt: time.Now().Add(-auth.ResendInterval - time.Second),
	})

	if _, err := svc.RequestCode(t.Context(), testPhone); err != nil {
		t.Errorf("RequestCode() error = %v, want nil once the gap has passed", err)
	}
}

func TestRequestCodeCountsOnlyTheCallersNumber(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)

	old := time.Now().Add(-5 * time.Minute)
	for range auth.MaxRequestsPerWindow {
		store.codes = append(store.codes, auth.OTPCode{
			ID: bson.NewObjectID(), Phone: "9000000000", CreatedAt: old,
		})
	}

	if _, err := svc.RequestCode(t.Context(), testPhone); err != nil {
		t.Errorf("RequestCode() error = %v; another number's traffic must not count", err)
	}
}

func TestRequestCodeSurfacesStoreFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*memStore)
	}{
		{name: "count fails", setup: func(m *memStore) { m.countErr = errors.New("boom") }},
		{name: "latest fails", setup: func(m *memStore) { m.latestErr = errors.New("boom") }},
		{name: "create fails", setup: func(m *memStore) { m.createErr = errors.New("boom") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemStore()
			tt.setup(store)

			if _, err := newService(t, store, &recordingSender{}, false).
				RequestCode(t.Context(), testPhone); err == nil {
				t.Error("RequestCode() error = nil, want the store failure to surface")
			}
		})
	}
}

func TestRequestCodeSurfacesSenderFailures(t *testing.T) {
	sender := &recordingSender{err: auth.ErrNoSender}

	_, err := newService(t, newMemStore(), sender, false).RequestCode(t.Context(), testPhone)
	if !errors.Is(err, auth.ErrNoSender) {
		t.Errorf("RequestCode() error = %v, want ErrNoSender to surface", err)
	}
}

/* ------------------------------------------------------------------- Verify */

// issue asks for a code and returns the plaintext that was "sent".
func issue(t *testing.T, svc *auth.Service, sender *recordingSender, phone string) string {
	t.Helper()
	if _, err := svc.RequestCode(t.Context(), phone); err != nil {
		t.Fatalf("RequestCode() error = %v", err)
	}
	return sender.code
}

func TestVerifyAcceptsTheCorrectCodeAndCreatesTheUser(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)
	code := issue(t, svc, sender, testPhone)

	result, err := svc.Verify(t.Context(), testPhone, code)
	if err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}
	if result.Token == "" {
		t.Error("Verify() returned no token")
	}
	if result.User.Phone != testPhone {
		t.Errorf("user phone = %q, want %q", result.User.Phone, testPhone)
	}
	if result.User.ID.IsZero() {
		t.Error("the created user has no id")
	}
	if !result.ExpiresAt.After(time.Now()) {
		t.Error("the token expires in the past")
	}
}

func TestVerifyRejectsAnyWrongSixDigitCode(t *testing.T) {
	// This is the gap the frontend had: it accepted any six digits.
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)
	code := issue(t, svc, sender, testPhone)

	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}

	if _, err := svc.Verify(t.Context(), testPhone, wrong); !errors.Is(err, auth.ErrCodeRejected) {
		t.Fatalf("Verify() error = %v, want ErrCodeRejected", err)
	}
	if store.attemptsLogged != 1 {
		t.Errorf("failed attempts recorded = %d, want 1; without this the lockout never bites",
			store.attemptsLogged)
	}
}

func TestVerifyIsSingleUse(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)
	code := issue(t, svc, sender, testPhone)

	if _, err := svc.Verify(t.Context(), testPhone, code); err != nil {
		t.Fatalf("first Verify() error = %v", err)
	}
	if _, err := svc.Verify(t.Context(), testPhone, code); !errors.Is(err, auth.ErrCodeRejected) {
		t.Errorf("second Verify() error = %v, want the code to be spent", err)
	}
}

func TestVerifyRejectsALostConsumeRace(t *testing.T) {
	// Two requests carrying the same correct code: only one may win.
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)
	code := issue(t, svc, sender, testPhone)

	lost := false
	store.consumeReturns = &lost

	if _, err := svc.Verify(t.Context(), testPhone, code); !errors.Is(err, auth.ErrCodeRejected) {
		t.Errorf("Verify() error = %v, want ErrCodeRejected when the consume is lost", err)
	}
}

func TestVerifyRejectsAnExpiredCode(t *testing.T) {
	store := newMemStore()
	store.codes = append(store.codes, auth.OTPCode{
		ID:        bson.NewObjectID(),
		Phone:     testPhone,
		CodeHash:  auth.HashCode(pepper, testPhone, "123456"),
		CreatedAt: time.Now().Add(-2 * auth.CodeTTL),
		ExpiresAt: time.Now().Add(-auth.CodeTTL),
	})

	svc := newService(t, store, &recordingSender{}, false)
	if _, err := svc.Verify(t.Context(), testPhone, "123456"); !errors.Is(err, auth.ErrCodeRejected) {
		t.Errorf("Verify() error = %v, want ErrCodeRejected for an expired code", err)
	}
}

func TestVerifyRejectsALockedOutCode(t *testing.T) {
	store := newMemStore()
	store.codes = append(store.codes, auth.OTPCode{
		ID:        bson.NewObjectID(),
		Phone:     testPhone,
		CodeHash:  auth.HashCode(pepper, testPhone, "123456"),
		Attempts:  auth.MaxAttempts,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(auth.CodeTTL),
	})

	svc := newService(t, store, &recordingSender{}, false)
	// Even the correct code must fail once the attempt budget is gone.
	if _, err := svc.Verify(t.Context(), testPhone, "123456"); !errors.Is(err, auth.ErrCodeRejected) {
		t.Errorf("Verify() error = %v, want ErrCodeRejected after lockout", err)
	}
}

func TestVerifyLocksOutAfterRepeatedGuesses(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)
	code := issue(t, svc, sender, testPhone)

	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}
	for range auth.MaxAttempts {
		if _, err := svc.Verify(t.Context(), testPhone, wrong); !errors.Is(err, auth.ErrCodeRejected) {
			t.Fatalf("Verify() error = %v, want ErrCodeRejected", err)
		}
	}

	// The right code no longer helps.
	if _, err := svc.Verify(t.Context(), testPhone, code); !errors.Is(err, auth.ErrCodeRejected) {
		t.Errorf("Verify() with the correct code = %v, want it locked out", err)
	}
}

func TestVerifyRejectsWhenNoCodeWasRequested(t *testing.T) {
	svc := newService(t, newMemStore(), &recordingSender{}, false)

	if _, err := svc.Verify(t.Context(), testPhone, "123456"); !errors.Is(err, auth.ErrCodeRejected) {
		t.Errorf("Verify() error = %v, want ErrCodeRejected", err)
	}
}

func TestVerifyRejectsMalformedInput(t *testing.T) {
	svc := newService(t, newMemStore(), &recordingSender{}, false)

	if _, err := svc.Verify(t.Context(), "123", "123456"); !errors.Is(err, auth.ErrInvalidPhone) {
		t.Errorf("Verify() error = %v, want ErrInvalidPhone", err)
	}
	for _, code := range []string{"", "12345", "1234567", "abcdef"} {
		if _, err := svc.Verify(t.Context(), testPhone, code); !errors.Is(err, auth.ErrInvalidCodeShape) {
			t.Errorf("Verify(code=%q) error = %v, want ErrInvalidCodeShape", code, err)
		}
	}
}

func TestVerifyReusesAnExistingUser(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)

	first, err := svc.Verify(t.Context(), testPhone, issue(t, svc, sender, testPhone))
	if err != nil {
		t.Fatalf("first Verify() error = %v", err)
	}

	// Clear the resend gate and sign in again.
	store.codes = nil
	second, err := svc.Verify(t.Context(), testPhone, issue(t, svc, sender, testPhone))
	if err != nil {
		t.Fatalf("second Verify() error = %v", err)
	}

	if first.User.ID != second.User.ID {
		t.Errorf("signing in twice created two users: %v and %v", first.User.ID, second.User.ID)
	}
}

func TestVerifySurfacesStoreFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*memStore)
	}{
		{name: "latest fails", setup: func(m *memStore) { m.latestErr = errors.New("boom") }},
		{name: "attempt fails", setup: func(m *memStore) { m.attemptErr = errors.New("boom") }},
		{name: "consume fails", setup: func(m *memStore) { m.consumeErr = errors.New("boom") }},
		{name: "upsert fails", setup: func(m *memStore) { m.upsertErr = errors.New("boom") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, sender := newMemStore(), &recordingSender{}
			svc := newService(t, store, sender, false)
			code := issue(t, svc, sender, testPhone)

			// Apply the failure only after the code exists.
			tt.setup(store)
			// "attempt fails" needs a wrong code to reach that branch.
			guess := code
			if tt.name == "attempt fails" {
				guess = "000000"
				if guess == code {
					guess = "111111"
				}
			}

			if _, err := svc.Verify(t.Context(), testPhone, guess); err == nil {
				t.Error("Verify() error = nil, want the store failure to surface")
			}
		})
	}
}

/* ----------------------------------------------------------------- UserByID */

func TestUserByID(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)

	result, err := svc.Verify(t.Context(), testPhone, issue(t, svc, sender, testPhone))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	got, err := svc.UserByID(t.Context(), result.User.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if got.Phone != testPhone {
		t.Errorf("phone = %q, want %q", got.Phone, testPhone)
	}

	if _, err := svc.UserByID(t.Context(), bson.NewObjectID()); !errors.Is(err, auth.ErrUserNotFound) {
		t.Errorf("UserByID(unknown) error = %v, want ErrUserNotFound", err)
	}
}

func TestParseTokenIsExposedForTheMiddleware(t *testing.T) {
	store, sender := newMemStore(), &recordingSender{}
	svc := newService(t, store, sender, false)

	result, err := svc.Verify(t.Context(), testPhone, issue(t, svc, sender, testPhone))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	claims, err := svc.ParseToken(result.Token)
	if err != nil {
		t.Fatalf("ParseToken() error = %v, want nil", err)
	}
	if claims.Subject != result.User.ID.Hex() {
		t.Errorf("subject = %q, want %q", claims.Subject, result.User.ID.Hex())
	}
}
