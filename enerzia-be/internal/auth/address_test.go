package auth_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/enerzia/enerzia-be/internal/auth"
)

// validAddress is the example from product.md §3.4.
func validAddress() auth.Address {
	return auth.Address{
		Name:  "Ananya Sharma",
		Email: "ananya@example.com",
		Phone: "9876543210",
		Line1: "12, Anand Residency, MG Road",
		City:  "Pune",
		State: "Maharashtra",
		Pin:   "411001",
	}
}

func TestValidateAddressAcceptsACompleteAddress(t *testing.T) {
	if _, ok := auth.ValidateAddress(validAddress()); !ok {
		t.Error("ValidateAddress() rejected a complete address")
	}
}

// TestValidateAddressAcceptsAnInternationalDeliveryPhone: the delivery ADDRESS
// stays Indian (six-digit PIN, Indian state), but the contact number on it may
// be foreign — an overseas shopper sending to family here is reachable on
// their own number, not an Indian one.
func TestValidateAddressAcceptsAnInternationalDeliveryPhone(t *testing.T) {
	for _, phone := range []string{"919876543210", "12025551234", "447700900123", "9876543210"} {
		t.Run(phone, func(t *testing.T) {
			addr := validAddress()
			addr.Phone = phone
			if problem, ok := auth.ValidateAddress(addr); !ok {
				t.Errorf("ValidateAddress() rejected phone %q: %s", phone, problem.Message)
			}
		})
	}
}

func TestValidateAddressMessagesMatchTheFrontend(t *testing.T) {
	// These strings are the contract with the shipped UI (product.md §3.4).
	// A reworded message here is a visible product change.
	tests := []struct {
		name        string
		mutate      func(*auth.Address)
		wantField   string
		wantMessage string
	}{
		{
			name:      "missing name",
			mutate:    func(a *auth.Address) { a.Name = "" },
			wantField: "name", wantMessage: "Please enter the name for delivery.",
		},
		{
			name:      "whitespace name",
			mutate:    func(a *auth.Address) { a.Name = "   " },
			wantField: "name", wantMessage: "Please enter the name for delivery.",
		},
		{
			name:      "email without an at sign",
			mutate:    func(a *auth.Address) { a.Email = "ananya.example.com" },
			wantField: "email", wantMessage: "Please enter a valid email for order updates.",
		},
		{
			name:      "email without a dot",
			mutate:    func(a *auth.Address) { a.Email = "ananya@example" },
			wantField: "email", wantMessage: "Please enter a valid email for order updates.",
		},
		{
			name:      "missing phone",
			mutate:    func(a *auth.Address) { a.Phone = "" },
			wantField: "phone", wantMessage: "Please enter a valid mobile number for delivery.",
		},
		{
			name:      "phone too short",
			mutate:    func(a *auth.Address) { a.Phone = "1234567" },
			wantField: "phone", wantMessage: "Please enter a valid mobile number for delivery.",
		},
		{
			// The '+' is trimmed only on the way in from MSG91; an address is
			// typed by a shopper and the form strips punctuation before it gets
			// here, so a '+' reaching this point is malformed.
			name:      "phone carries a plus",
			mutate:    func(a *auth.Address) { a.Phone = "+919876543210" },
			wantField: "phone", wantMessage: "Please enter a valid mobile number for delivery.",
		},
		{
			name:      "phone with separators",
			mutate:    func(a *auth.Address) { a.Phone = "98765 43210" },
			wantField: "phone", wantMessage: "Please enter a valid mobile number for delivery.",
		},
		{
			name:      "street too short",
			mutate:    func(a *auth.Address) { a.Line1 = "12" },
			wantField: "line1", wantMessage: "Please enter your full street address.",
		},
		{
			name:      "street is only spaces",
			mutate:    func(a *auth.Address) { a.Line1 = "          " },
			wantField: "line1", wantMessage: "Please enter your full street address.",
		},
		{
			name:      "missing city",
			mutate:    func(a *auth.Address) { a.City = "" },
			wantField: "city", wantMessage: "Please enter your city and state.",
		},
		{
			name:      "missing state",
			mutate:    func(a *auth.Address) { a.State = " " },
			wantField: "city", wantMessage: "Please enter your city and state.",
		},
		{
			name:      "pin too short",
			mutate:    func(a *auth.Address) { a.Pin = "41100" },
			wantField: "pin", wantMessage: "PIN code must be 6 digits.",
		},
		{
			name:      "pin with letters",
			mutate:    func(a *auth.Address) { a.Pin = "4110o1" },
			wantField: "pin", wantMessage: "PIN code must be 6 digits.",
		},
		{
			name:      "pin too long",
			mutate:    func(a *auth.Address) { a.Pin = "4110011" },
			wantField: "pin", wantMessage: "PIN code must be 6 digits.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := validAddress()
			tt.mutate(&addr)

			problem, ok := auth.ValidateAddress(addr)
			if ok {
				t.Fatalf("ValidateAddress() accepted %+v", addr)
			}
			if problem.Field != tt.wantField {
				t.Errorf("field = %q, want %q", problem.Field, tt.wantField)
			}
			if problem.Message != tt.wantMessage {
				t.Errorf("message = %q, want the frontend's exact wording %q",
					problem.Message, tt.wantMessage)
			}
		})
	}
}

func TestValidateAddressReportsFieldsInTheFrontendsOrder(t *testing.T) {
	// Every field is wrong at once. The UI shows one message, so the API must
	// pick the same one it would have — the first in the sequence.
	empty := auth.Address{}

	problem, ok := auth.ValidateAddress(empty)
	if ok {
		t.Fatal("ValidateAddress() accepted an empty address")
	}
	if problem.Field != "name" {
		t.Errorf("first failure = %q, want name — the order is the contract", problem.Field)
	}

	// Fixing each in turn should walk the sequence exactly.
	wantOrder := []string{"name", "email", "phone", "line1", "city", "pin"}
	addr := auth.Address{}
	fix := []func(*auth.Address){
		func(a *auth.Address) { a.Name = "Ananya" },
		func(a *auth.Address) { a.Email = "a@b.co" },
		func(a *auth.Address) { a.Phone = "9876543210" },
		func(a *auth.Address) { a.Line1 = "12, MG Road" },
		func(a *auth.Address) { a.City, a.State = "Pune", "Maharashtra" },
		func(a *auth.Address) { a.Pin = "411001" },
	}
	for i, want := range wantOrder {
		problem, ok := auth.ValidateAddress(addr)
		if ok {
			t.Fatalf("step %d: address accepted too early", i)
		}
		if problem.Field != want {
			t.Errorf("step %d: field = %q, want %q", i, problem.Field, want)
		}
		fix[i](&addr)
	}
	if _, ok := auth.ValidateAddress(addr); !ok {
		t.Error("the address is still rejected after every field was fixed")
	}
}

/* --------------------------------------- /api/v1/me/addresses */

type addressBody struct {
	Data struct {
		Address   auth.Address   `json:"address"`
		Addresses []auth.Address `json:"addresses"`
	} `json:"data"`
}

func decodeAddresses(t *testing.T, rec *httptest.ResponseRecorder) addressBody {
	t.Helper()
	var body addressBody
	decodeJSONInto(t, rec, &body)
	return body
}

const goodAddress = `{
	"name":"Ananya Sharma","email":"ananya@example.com","phone":"9876543210",
	"line1":"12, Anand Residency, MG Road",
	"city":"Pune","state":"Maharashtra","pin":"411001"}`

func TestListAddressesIsEmptyWhenNoneAreSaved(t *testing.T) {
	// Not a 404: a blank address book is a state, not an error.
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	rec := getWithToken(t, h, "/api/v1/me/addresses", "Bearer "+token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"addresses":[]`) {
		t.Errorf("body = %s, want an empty array", rec.Body.String())
	}
}

func TestAddAddressCreatesAndDefaults(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	rec := postWithToken(t, h, "/api/v1/me/addresses", token, goodAddress)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	saved := decodeAddresses(t, rec).Data.Address
	if saved.ID.IsZero() {
		t.Error("the created address has no id")
	}
	// The first address must be the default, or checkout has nothing selected.
	if !saved.IsDefault {
		t.Error("the first address saved should become the default")
	}
	if saved.City != "Pune" {
		t.Errorf("city = %q", saved.City)
	}
}

func TestAddAddressTrimsWhitespace(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	rec := postWithToken(t, h, "/api/v1/me/addresses", token, `{
		"name":"  Ananya Sharma  ","email":" ananya@example.com ","phone":" 9876543210 ",
		"line1":" 12, Anand Residency ","city":" Pune ",
		"state":" Maharashtra ","pin":" 411001 "}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	saved := decodeAddresses(t, rec).Data.Address
	if saved.Name != "Ananya Sharma" || saved.Pin != "411001" || saved.Phone != "9876543210" {
		t.Errorf("address not trimmed: %+v", saved)
	}
}

func TestSecondAddressDoesNotStealTheDefault(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	postWithToken(t, h, "/api/v1/me/addresses", token, goodAddress)
	rec := postWithToken(t, h, "/api/v1/me/addresses", token, `{
		"label":"Office","name":"Ananya Sharma","email":"ananya@example.com","phone":"9876543210",
		"line1":"5, Tech Park Road","city":"Pune","state":"Maharashtra","pin":"411014"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if decodeAddresses(t, rec).Data.Address.IsDefault {
		t.Error("a later address became the default without being asked")
	}

	list := decodeAddresses(t, getWithToken(t, h, "/api/v1/me/addresses", "Bearer "+token)).Data.Addresses
	if len(list) != 2 {
		t.Fatalf("%d addresses, want 2", len(list))
	}
	// Default first, so the checkout form can take addresses[0].
	if !list[0].IsDefault {
		t.Error("the list is not default-first")
	}
}

func TestAskingForDefaultMovesIt(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	postWithToken(t, h, "/api/v1/me/addresses", token, goodAddress)
	rec := postWithToken(t, h, "/api/v1/me/addresses", token, `{
		"label":"Office","name":"Ananya Sharma","email":"ananya@example.com","phone":"9876543210",
		"line1":"5, Tech Park Road","city":"Pune","state":"Maharashtra",
		"pin":"411014","isDefault":true}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}

	list := decodeAddresses(t, getWithToken(t, h, "/api/v1/me/addresses", "Bearer "+token)).Data.Addresses
	defaults := 0
	for _, a := range list {
		if a.IsDefault {
			defaults++
			if a.Label != "Office" {
				t.Errorf("default is %q, want Office", a.Label)
			}
		}
	}
	// Exactly one default is the invariant Mongo cannot express.
	if defaults != 1 {
		t.Errorf("%d defaults, want exactly 1", defaults)
	}
}

func TestUpdateAddressReplacesIt(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	created := decodeAddresses(t, postWithToken(t, h, "/api/v1/me/addresses", token, goodAddress)).Data.Address

	rec := putJSON(t, h, "/api/v1/me/addresses/"+created.ID.Hex(), token, `{
		"name":"Ananya S","email":"ananya@example.com","phone":"9876543210",
		"line1":"99, New Street Road","city":"Mumbai","state":"Maharashtra","pin":"400001"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	updated := decodeAddresses(t, rec).Data.Address
	if updated.City != "Mumbai" || updated.ID != created.ID {
		t.Errorf("updated = %+v, want the same id with new content", updated)
	}
	// Clearing the flag on the only default is ignored.
	if !updated.IsDefault {
		t.Error("the only address stopped being the default")
	}
}

func TestUpdateUnknownAddressIs404(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	rec := putJSON(t, h, "/api/v1/me/addresses/"+bson.NewObjectID().Hex(), token, goodAddress)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestMalformedAddressIDIs404(t *testing.T) {
	// From the caller's view an unparseable id is simply one that does not
	// exist; a 400 would leak that ids have a format.
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	if rec := putJSON(t, h, "/api/v1/me/addresses/nonsense", token, goodAddress); rec.Code != http.StatusNotFound {
		t.Errorf("PUT status = %d, want 404", rec.Code)
	}
	if rec := deleteWithToken(t, h, "/api/v1/me/addresses/nonsense", token); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE status = %d, want 404", rec.Code)
	}
}

func TestDeleteAddressPromotesTheNextDefault(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	first := decodeAddresses(t, postWithToken(t, h, "/api/v1/me/addresses", token, goodAddress)).Data.Address
	postWithToken(t, h, "/api/v1/me/addresses", token, `{
		"label":"Office","name":"Ananya Sharma","email":"ananya@example.com","phone":"9876543210",
		"line1":"5, Tech Park Road","city":"Pune","state":"Maharashtra","pin":"411014"}`)

	rec := deleteWithToken(t, h, "/api/v1/me/addresses/"+first.ID.Hex(), token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	remaining := decodeAddresses(t, rec).Data.Addresses
	if len(remaining) != 1 {
		t.Fatalf("%d addresses left, want 1", len(remaining))
	}
	// A shopper with addresses always has one selected.
	if !remaining[0].IsDefault {
		t.Error("deleting the default left nobody selected")
	}
}

func TestDeleteLastAddressLeavesAnEmptyList(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	created := decodeAddresses(t, postWithToken(t, h, "/api/v1/me/addresses", token, goodAddress)).Data.Address

	rec := deleteWithToken(t, h, "/api/v1/me/addresses/"+created.ID.Hex(), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"addresses":[]`) {
		t.Errorf("body = %s, want an empty array", rec.Body.String())
	}
}

func TestDeleteUnknownAddressIs404(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	rec := deleteWithToken(t, h, "/api/v1/me/addresses/"+bson.NewObjectID().Hex(), token)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAddressesAreIsolatedPerShopper(t *testing.T) {
	store := newMemStore()
	h := newAuthAPI(t, store, &recordingSender{}, true)

	tokenA := signIn(t, h, "9876543210")
	postWithToken(t, h, "/api/v1/me/addresses", tokenA, goodAddress)

	// A second shopper must not see the first one's address book.
	store.codes = nil
	tokenB := signIn(t, h, "9000000000")

	list := decodeAddresses(t, getWithToken(t, h, "/api/v1/me/addresses", "Bearer "+tokenB)).Data.Addresses
	if len(list) != 0 {
		t.Errorf("%d addresses visible to another shopper", len(list))
	}
}

func TestAddAddressRejectsAnIncompleteAddress(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	rec := postWithToken(t, h, "/api/v1/me/addresses", token, `{
		"name":"Ananya Sharma","email":"not-an-email",
		"line1":"12, Anand Residency, MG Road",
		"city":"Pune","state":"Maharashtra","pin":"411001"}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeJSONInto(t, rec, &body)
	if body.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("error.code = %q, want VALIDATION_ERROR", body.Error.Code)
	}
	if body.Error.Message != "Please enter a valid email for order updates." {
		t.Errorf("message = %q, want the frontend's wording", body.Error.Message)
	}
}

func TestRejectedAddressDoesNotPartiallySave(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	postWithToken(t, h, "/api/v1/me/addresses", token, goodAddress)
	postWithToken(t, h, "/api/v1/me/addresses", token,
		`{"name":"Someone Else","email":"bad","line1":"x","city":"","state":"","pin":"1"}`)

	list := decodeAddresses(t, getWithToken(t, h, "/api/v1/me/addresses", "Bearer "+token)).Data.Addresses
	if len(list) != 1 || list[0].Name != "Ananya Sharma" {
		t.Errorf("addresses = %+v, want the rejected write to have changed nothing", list)
	}
}

func TestAddressRoutesRequireAuth(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	id := bson.NewObjectID().Hex()

	if rec := getWithToken(t, h, "/api/v1/me/addresses", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET = %d, want 401", rec.Code)
	}
	if rec := postWithToken(t, h, "/api/v1/me/addresses", "", goodAddress); rec.Code != http.StatusUnauthorized {
		t.Errorf("POST = %d, want 401", rec.Code)
	}
	if rec := putJSON(t, h, "/api/v1/me/addresses/"+id, "", goodAddress); rec.Code != http.StatusUnauthorized {
		t.Errorf("PUT = %d, want 401", rec.Code)
	}
	if rec := deleteWithToken(t, h, "/api/v1/me/addresses/"+id, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("DELETE = %d, want 401", rec.Code)
	}
}

func TestAddAddressRejectsAMalformedBody(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	for _, body := range []string{`{`, `nope`, `{"unknownField":1}`} {
		rec := postWithToken(t, h, "/api/v1/me/addresses", token, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q → status %d, want 400", body, rec.Code)
		}
	}
}

func TestAddressEndpointsHideInternalFailures(t *testing.T) {
	store := newMemStore()
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	store.setAddressErr = errors.New("cluster0-shard-00.mongodb.net timed out")
	rec := postWithToken(t, h, "/api/v1/me/addresses", token, goodAddress)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "mongodb.net") {
		t.Errorf("response leaks infrastructure detail: %s", rec.Body.String())
	}
}

func TestAddressEndpointsRejectAStaleToken(t *testing.T) {
	store := newMemStore()
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	store.users = map[string]auth.User{} // account removed, token still valid

	if rec := getWithToken(t, h, "/api/v1/me/addresses", "Bearer "+token); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET = %d, want 401", rec.Code)
	}
	if rec := postWithToken(t, h, "/api/v1/me/addresses", token, goodAddress); rec.Code != http.StatusUnauthorized {
		t.Errorf("POST = %d, want 401", rec.Code)
	}
}

func TestAddressRoutesRejectWrongMethods(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	if rec := putJSON(t, h, "/api/v1/me/addresses", token, goodAddress); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT on the collection = %d, want 405", rec.Code)
	}
}

/* ------------------------------------------------- AddressFor (for orders) */

// saveAddress posts an address and returns it.
func saveAddress(t *testing.T, h http.Handler, token, body string) auth.Address {
	t.Helper()
	rec := postWithToken(t, h, "/api/v1/me/addresses", token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("saving the address: status %d (%s)", rec.Code, rec.Body.String())
	}
	return decodeAddresses(t, rec).Data.Address
}

func TestAddressForPicksTheDefaultWhenNoneIsNamed(t *testing.T) {
	store := newMemStore()
	svc := newService(t, store, &recordingSender{}, false)
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	saveAddress(t, h, token, goodAddress)
	office := saveAddress(t, h, token, `{
		"label":"Office","name":"Ananya Sharma","email":"ananya@example.com","phone":"9876543210",
		"line1":"5, Tech Park Road","city":"Pune","state":"Maharashtra",
		"pin":"411014","isDefault":true}`)

	userID := store.users["9876543210"].ID

	got, _, err := svc.AddressFor(t.Context(), userID, nil)
	if err != nil {
		t.Fatalf("AddressFor(nil) error = %v", err)
	}
	if got.ID != office.ID {
		t.Errorf("AddressFor(nil) = %q, want the default (Office)", got.Label)
	}
}

func TestAddressForHonoursAnExplicitChoice(t *testing.T) {
	store := newMemStore()
	svc := newService(t, store, &recordingSender{}, false)
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")

	home := saveAddress(t, h, token, goodAddress)
	saveAddress(t, h, token, `{
		"label":"Office","name":"Ananya Sharma","email":"ananya@example.com","phone":"9876543210",
		"line1":"5, Tech Park Road","city":"Pune","state":"Maharashtra",
		"pin":"411014","isDefault":true}`)

	userID := store.users["9876543210"].ID

	got, _, err := svc.AddressFor(t.Context(), userID, &home.ID)
	if err != nil {
		t.Fatalf("AddressFor(home) error = %v", err)
	}
	if got.ID != home.ID {
		t.Errorf("AddressFor() picked %q, want the one named", got.Label)
	}
}

func TestAddressForRejectsAnUnknownID(t *testing.T) {
	store := newMemStore()
	svc := newService(t, store, &recordingSender{}, false)
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")
	saveAddress(t, h, token, goodAddress)

	userID := store.users["9876543210"].ID
	other := bson.NewObjectID()

	if _, _, err := svc.AddressFor(t.Context(), userID, &other); !errors.Is(err, auth.ErrAddressNotFound) {
		t.Errorf("AddressFor() error = %v, want ErrAddressNotFound", err)
	}
}

func TestAddressForWithNoAddressesAtAll(t *testing.T) {
	store := newMemStore()
	svc := newService(t, store, &recordingSender{}, false)
	h := newAuthAPI(t, store, &recordingSender{}, true)
	signIn(t, h, "9876543210")

	userID := store.users["9876543210"].ID

	if _, _, err := svc.AddressFor(t.Context(), userID, nil); !errors.Is(err, auth.ErrAddressNotFound) {
		t.Errorf("AddressFor() error = %v, want ErrAddressNotFound", err)
	}
}

func TestAddressForFallsBackWhenNoneIsFlagged(t *testing.T) {
	// Defensive: stored data with no default should still ship somewhere.
	store := newMemStore()
	svc := newService(t, store, &recordingSender{}, false)
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")
	saveAddress(t, h, token, goodAddress)

	u := store.users["9876543210"]
	u.Addresses[0].IsDefault = false
	store.users["9876543210"] = u

	got, _, err := svc.AddressFor(t.Context(), u.ID, nil)
	if err != nil {
		t.Fatalf("AddressFor() error = %v", err)
	}
	if got.ID != u.Addresses[0].ID {
		t.Error("AddressFor() did not fall back to the first address")
	}
}

func TestUpdateAddressSurfacesStoreFailures(t *testing.T) {
	store := newMemStore()
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")
	created := saveAddress(t, h, token, goodAddress)

	store.setAddressErr = errors.New("boom")
	rec := putJSON(t, h, "/api/v1/me/addresses/"+created.ID.Hex(), token, goodAddress)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestUpdateAddressRejectsAnInvalidBody(t *testing.T) {
	h := newAuthAPI(t, newMemStore(), &recordingSender{}, true)
	token := signIn(t, h, "9876543210")
	created := saveAddress(t, h, token, goodAddress)

	rec := putJSON(t, h, "/api/v1/me/addresses/"+created.ID.Hex(), token,
		`{"name":"","email":"a@b.co","line1":"12, MG Road","city":"Pune","state":"MH","pin":"411001"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
}

func TestDeleteAddressSurfacesStoreFailures(t *testing.T) {
	store := newMemStore()
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")
	created := saveAddress(t, h, token, goodAddress)

	store.setAddressErr = errors.New("boom")
	rec := deleteWithToken(t, h, "/api/v1/me/addresses/"+created.ID.Hex(), token)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestDeleteAddressRejectsAStaleToken(t *testing.T) {
	store := newMemStore()
	h := newAuthAPI(t, store, &recordingSender{}, true)
	token := signIn(t, h, "9876543210")
	created := saveAddress(t, h, token, goodAddress)

	store.users = map[string]auth.User{}

	rec := deleteWithToken(t, h, "/api/v1/me/addresses/"+created.ID.Hex(), token)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
