package auth_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/enerzia/enerzia-be/internal/auth"
	"github.com/enerzia/enerzia-be/internal/mongotest"
)

const (
	usersNS = "enerzia_test.users"
	codesNS = "enerzia_test.otp_codes"
)

// newAuthRepo wires a repository to a fake MongoDB. The fake stores nothing,
// so tests declare the reply then assert on the command that was sent.
func newAuthRepo(t *testing.T) (*auth.Repository, *mongotest.Server) {
	t.Helper()

	fake := mongotest.Start(t)
	client, err := mongo.Connect(options.Client().ApplyURI(fake.URI()).SetTimeout(5 * time.Second))
	if err != nil {
		t.Fatalf("connect to fake: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(t.Context()) })

	return auth.NewRepository(client.Database("enerzia_test")), fake
}

func codeDoc(phone string, attempts int, consumed bool) bson.D {
	now := time.Now()
	return bson.D{
		{Key: "_id", Value: bson.NewObjectID()},
		{Key: "phone", Value: phone},
		{Key: "codeHash", Value: "deadbeef"},
		{Key: "attempts", Value: attempts},
		{Key: "consumed", Value: consumed},
		{Key: "createdAt", Value: now},
		{Key: "expiresAt", Value: now.Add(auth.CodeTTL)},
	}
}

func TestCreateCodeInsertsTheChallenge(t *testing.T) {
	repo, fake := newAuthRepo(t)

	code := auth.OTPCode{
		Phone:     testPhone,
		CodeHash:  auth.HashCode(pepper, testPhone, "123456"),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(auth.CodeTTL),
	}
	if err := repo.CreateCode(t.Context(), code); err != nil {
		t.Fatalf("CreateCode() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("insert")
	if !ok {
		t.Fatal("no insert command was sent")
	}
	if got := req.Doc.Lookup("insert").StringValue(); got != "otp_codes" {
		t.Errorf("insert collection = %q, want otp_codes", got)
	}

	docs := req.Sequence("documents")
	if len(docs) != 1 {
		t.Fatalf("inserted %d documents, want 1", len(docs))
	}
	// The plaintext code must never reach the database.
	if strings.Contains(docs[0].String(), "123456") {
		t.Errorf("the inserted document contains the plaintext code: %s", docs[0].String())
	}
	if got := docs[0].Lookup("codeHash").StringValue(); got != code.CodeHash {
		t.Errorf("codeHash = %q, want the keyed hash", got)
	}
}

func TestCreateCodeWrapsErrors(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("insert", mongotest.Fail("not authorized", 13))

	err := repo.CreateCode(t.Context(), auth.OTPCode{Phone: testPhone})
	if err == nil {
		t.Fatal("CreateCode() error = nil, want the database failure to surface")
	}
	if !strings.Contains(err.Error(), "auth: create code") {
		t.Errorf("error = %q, want it wrapped", err)
	}
}

func TestLatestCodeTakesTheNewest(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("find", mongotest.Cursor(codesNS, codeDoc(testPhone, 0, false)))

	got, err := repo.LatestCode(t.Context(), testPhone)
	if err != nil {
		t.Fatalf("LatestCode() error = %v, want nil", err)
	}
	if got.Phone != testPhone {
		t.Errorf("phone = %q, want %q", got.Phone, testPhone)
	}

	req, _ := fake.LastRequest("find")
	if p := req.Doc.Lookup("filter").Document().Lookup("phone").StringValue(); p != testPhone {
		t.Errorf("filter.phone = %q, want %q", p, testPhone)
	}
	// Newest first, or a resend would be checked against a stale code.
	sortVal, err := req.Doc.LookupErr("sort")
	if err != nil {
		t.Fatalf("find carried no sort: %v", err)
	}
	if got := sortVal.Document().Lookup("createdAt").AsInt64(); got != -1 {
		t.Errorf("sort.createdAt = %d, want -1", got)
	}
}

func TestLatestCodeReportsNotFound(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("find", mongotest.Cursor(codesNS))

	if _, err := repo.LatestCode(t.Context(), testPhone); !errors.Is(err, auth.ErrCodeNotFound) {
		t.Errorf("LatestCode() error = %v, want ErrCodeNotFound", err)
	}
}

func TestLatestCodeWrapsErrors(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("find", mongotest.Fail("not authorized", 13))

	_, err := repo.LatestCode(t.Context(), testPhone)
	if err == nil || errors.Is(err, auth.ErrCodeNotFound) {
		t.Fatalf("LatestCode() error = %v, want a wrapped database failure", err)
	}
}

func TestCountCodesSinceFiltersByPhoneAndWindow(t *testing.T) {
	repo, fake := newAuthRepo(t)
	// CountDocuments runs an aggregate under the hood.
	fake.Respond("aggregate", mongotest.Cursor(codesNS, bson.D{{Key: "n", Value: int32(2)}}))

	since := time.Now().Add(-auth.RateWindow)
	n, err := repo.CountCodesSince(t.Context(), testPhone, since)
	if err != nil {
		t.Fatalf("CountCodesSince() error = %v, want nil", err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
}

func TestCountCodesSinceWrapsErrors(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("aggregate", mongotest.Fail("not authorized", 13))

	if _, err := repo.CountCodesSince(t.Context(), testPhone, time.Now()); err == nil {
		t.Fatal("CountCodesSince() error = nil, want the failure to surface")
	}
}

func TestRecordFailedAttemptIncrements(t *testing.T) {
	repo, fake := newAuthRepo(t)
	id := bson.NewObjectID()

	if err := repo.RecordFailedAttempt(t.Context(), id); err != nil {
		t.Fatalf("RecordFailedAttempt() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	updates := req.Sequence("updates")
	if len(updates) == 0 {
		t.Fatal("no update entry")
	}
	inc := updates[0].Lookup("u").Document().Lookup("$inc").Document()
	if got := inc.Lookup("attempts").AsInt64(); got != 1 {
		t.Errorf("$inc.attempts = %d, want 1", got)
	}
}

func TestRecordFailedAttemptWrapsErrors(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	if err := repo.RecordFailedAttempt(t.Context(), bson.NewObjectID()); err == nil {
		t.Fatal("RecordFailedAttempt() error = nil, want the failure to surface")
	}
}

func TestConsumeCodeGuardsAgainstReplay(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 1}, {Key: "nModified", Value: 1}, {Key: "ok", Value: 1},
	}))

	ok, err := repo.ConsumeCode(t.Context(), bson.NewObjectID())
	if err != nil {
		t.Fatalf("ConsumeCode() error = %v, want nil", err)
	}
	if !ok {
		t.Error("ConsumeCode() = false, want true when one document was modified")
	}

	req, _ := fake.LastRequest("update")
	filter := req.Sequence("updates")[0].Lookup("q").Document()
	// The consumed:false clause is what makes two concurrent verifications
	// mutually exclusive.
	if consumed, err := filter.LookupErr("consumed"); err != nil || consumed.Boolean() {
		t.Error("the update filter must require consumed:false")
	}
}

func TestConsumeCodeReportsALostRace(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 0}, {Key: "nModified", Value: 0}, {Key: "ok", Value: 1},
	}))

	ok, err := repo.ConsumeCode(t.Context(), bson.NewObjectID())
	if err != nil {
		t.Fatalf("ConsumeCode() error = %v, want nil", err)
	}
	if ok {
		t.Error("ConsumeCode() = true when nothing was modified; a replay would succeed")
	}
}

func TestConsumeCodeWrapsErrors(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	if _, err := repo.ConsumeCode(t.Context(), bson.NewObjectID()); err == nil {
		t.Fatal("ConsumeCode() error = nil, want the failure to surface")
	}
}

// userDoc is a users document as the fake returns it from findAndModify.
func userDoc(id bson.ObjectID, phone string, now time.Time) bson.D {
	return bson.D{
		{Key: "value", Value: bson.D{
			{Key: "_id", Value: id},
			{Key: "phone", Value: phone},
			{Key: "createdAt", Value: now},
			{Key: "updatedAt", Value: now},
		}},
		{Key: "ok", Value: 1},
	}
}

// noDoc is findAndModify's "matched nothing" reply, which the driver turns
// into mongo.ErrNoDocuments.
func noDoc() bson.D {
	return bson.D{{Key: "value", Value: nil}, {Key: "ok", Value: 1}}
}

// findAndModifyCount counts the probes UpsertUser actually issued. The number
// matters: an extra probe is a query per sign-in for every shopper.
func findAndModifyCount(fake *mongotest.Server) int {
	n := 0
	for _, c := range fake.Commands() {
		if c == "findAndModify" {
			n++
		}
	}
	return n
}

func TestUpsertUserReturnsAnExistingUserWithoutUpserting(t *testing.T) {
	repo, fake := newAuthRepo(t)
	now := time.Now()
	const e164 = "919876543210"
	fake.Respond("findAndModify", mongotest.Reply(userDoc(bson.NewObjectID(), e164, now)))

	user, err := repo.UpsertUser(t.Context(), e164, now)
	if err != nil {
		t.Fatalf("UpsertUser() error = %v, want nil", err)
	}
	if user.Phone != e164 {
		t.Errorf("phone = %q, want %q", user.Phone, e164)
	}
	if n := findAndModifyCount(fake); n != 1 {
		t.Errorf("findAndModify called %d times, want 1 — a known shopper must cost one query", n)
	}
	req, _ := fake.LastRequest("findAndModify")
	// The first probe must NOT upsert. If it did, a legacy ten-digit shopper
	// would get a second, empty account before the migration ever ran.
	if _, err := req.Doc.LookupErr("upsert"); err == nil {
		t.Error("the first probe must not upsert, or the legacy lookup is unreachable")
	}
}

func TestUpsertUserCreatesOnFirstSignIn(t *testing.T) {
	repo, fake := newAuthRepo(t)
	now := time.Now()
	const e164 = "919876543210"
	// miss on the new form, miss on the legacy form, then the insert.
	fake.RespondSequence("findAndModify",
		mongotest.Reply(noDoc()),
		mongotest.Reply(noDoc()),
		mongotest.Reply(userDoc(bson.NewObjectID(), e164, now)),
	)

	user, err := repo.UpsertUser(t.Context(), e164, now)
	if err != nil {
		t.Fatalf("UpsertUser() error = %v, want nil", err)
	}
	if user.Phone != e164 {
		t.Errorf("phone = %q, want %q", user.Phone, e164)
	}
	if user.ID.IsZero() {
		t.Error("the returned user has no id")
	}

	req, ok := fake.LastRequest("findAndModify")
	if !ok {
		t.Fatal("no findAndModify command was sent")
	}
	if upsert, err := req.Doc.LookupErr("upsert"); err != nil || !upsert.Boolean() {
		t.Error("UpsertUser must upsert, or a first-time shopper cannot sign in")
	}
	if newVal, err := req.Doc.LookupErr("new"); err != nil || !newVal.Boolean() {
		t.Error("UpsertUser must return the document after the write")
	}
	// createdAt must only be set on insert, or a returning shopper's
	// join date is rewritten on every sign-in.
	setOnInsert := req.Doc.Lookup("update").Document().Lookup("$setOnInsert").Document()
	if _, err := setOnInsert.LookupErr("createdAt"); err != nil {
		t.Error("createdAt must be under $setOnInsert")
	}
}

// TestUpsertUserMigratesALegacyDocumentInPlace is the heart of the lazy
// migration (schema.md §users). The _id must survive: carts, addresses and
// orders all reference the user by _id, so an insert instead of a rename would
// strand every one of them against a second, empty account.
func TestUpsertUserMigratesALegacyDocumentInPlace(t *testing.T) {
	repo, fake := newAuthRepo(t)
	now := time.Now()
	existing := bson.NewObjectID()
	const legacy = "9876543210"
	const e164 = "91" + legacy

	fake.RespondSequence("findAndModify",
		mongotest.Reply(noDoc()),                      // nothing at the new form
		mongotest.Reply(userDoc(existing, e164, now)), // the legacy doc, renamed
	)

	user, err := repo.UpsertUser(t.Context(), e164, now)
	if err != nil {
		t.Fatalf("UpsertUser() error = %v, want nil", err)
	}
	if user.ID != existing {
		t.Errorf("id = %v, want %v — migrating must keep the _id, or the shopper loses their orders", user.ID, existing)
	}
	if user.Phone != e164 {
		t.Errorf("phone = %q, want %q", user.Phone, e164)
	}
	if n := findAndModifyCount(fake); n != 2 {
		t.Fatalf("findAndModify called %d times, want 2 (new form, then legacy)", n)
	}

	req, _ := fake.LastRequest("findAndModify")
	// It must look up the TEN-DIGIT form...
	if q := req.Doc.Lookup("query").Document().Lookup("phone").StringValue(); q != legacy {
		t.Errorf("legacy lookup queried phone %q, want %q", q, legacy)
	}
	// ...and rewrite it to the new one.
	set := req.Doc.Lookup("update").Document().Lookup("$set").Document()
	if got := set.Lookup("phone").StringValue(); got != e164 {
		t.Errorf("migration set phone = %q, want %q", got, e164)
	}
	// It must never insert: an upsert here would create the second account the
	// whole migration exists to avoid.
	if _, err := req.Doc.LookupErr("upsert"); err == nil {
		t.Error("the legacy lookup must not upsert")
	}
}

// TestUpsertUserSkipsTheLegacyLookupForForeignNumbers guards the cost of the
// migration: only Indian numbers ever had a ten-digit form, so nobody else
// should pay a query for it.
func TestUpsertUserSkipsTheLegacyLookupForForeignNumbers(t *testing.T) {
	repo, fake := newAuthRepo(t)
	now := time.Now()
	const us = "12025551234"
	fake.RespondSequence("findAndModify",
		mongotest.Reply(noDoc()),
		mongotest.Reply(userDoc(bson.NewObjectID(), us, now)),
	)

	if _, err := repo.UpsertUser(t.Context(), us, now); err != nil {
		t.Fatalf("UpsertUser() error = %v, want nil", err)
	}
	if n := findAndModifyCount(fake); n != 2 {
		t.Errorf("findAndModify called %d times, want 2 — a foreign number has no legacy form to probe", n)
	}
	req, _ := fake.LastRequest("findAndModify")
	if upsert, err := req.Doc.LookupErr("upsert"); err != nil || !upsert.Boolean() {
		t.Error("the second call for a foreign number must be the creating upsert")
	}
}

// TestUpsertUserSkipsTheLegacyLookupForAnOddLengthIndianPrefix covers a number
// that merely starts with 91 without being 91 + ten digits.
func TestUpsertUserSkipsTheLegacyLookupForAnOddLengthIndianPrefix(t *testing.T) {
	repo, fake := newAuthRepo(t)
	now := time.Now()
	const notIndian = "9112345678" // starts 91, but only eight digits follow
	fake.RespondSequence("findAndModify",
		mongotest.Reply(noDoc()),
		mongotest.Reply(userDoc(bson.NewObjectID(), notIndian, now)),
	)

	if _, err := repo.UpsertUser(t.Context(), notIndian, now); err != nil {
		t.Fatalf("UpsertUser() error = %v, want nil", err)
	}
	if n := findAndModifyCount(fake); n != 2 {
		t.Errorf("findAndModify called %d times, want 2", n)
	}
}

// TestUpsertUserResolvesAMigrationRace covers a document appearing at the new
// form between the first probe and the rename. The rename collides with the
// unique index; the winner is the document we wanted, so re-read it rather
// than failing a sign-in over a race we can resolve.
func TestUpsertUserResolvesAMigrationRace(t *testing.T) {
	repo, fake := newAuthRepo(t)
	now := time.Now()
	winner := bson.NewObjectID()
	const e164 = "919876543210"

	fake.RespondSequence("findAndModify",
		mongotest.Reply(noDoc()),
		mongotest.Fail("E11000 duplicate key error collection: enerzia.users index: phone_1", 11000),
		mongotest.Reply(userDoc(winner, e164, now)),
	)

	user, err := repo.UpsertUser(t.Context(), e164, now)
	if err != nil {
		t.Fatalf("UpsertUser() error = %v, want nil — a resolvable race must not fail sign-in", err)
	}
	if user.ID != winner {
		t.Errorf("id = %v, want the winning document %v", user.ID, winner)
	}
}

// TestUpsertUserSurfacesAFailedRaceRecheck stops the race branch swallowing a
// real database failure.
func TestUpsertUserSurfacesAFailedRaceRecheck(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.RespondSequence("findAndModify",
		mongotest.Reply(noDoc()),
		mongotest.Fail("E11000 duplicate key error collection: enerzia.users index: phone_1", 11000),
		mongotest.Fail("not authorized", 13),
	)

	if _, err := repo.UpsertUser(t.Context(), "919876543210", time.Now()); err == nil {
		t.Fatal("UpsertUser() error = nil, want the failure to surface")
	}
}

// TestUpsertUserSurfacesAFailedLegacyLookup keeps a driver error on the
// migration probe from being mistaken for "no legacy document".
func TestUpsertUserSurfacesAFailedLegacyLookup(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.RespondSequence("findAndModify",
		mongotest.Reply(noDoc()),
		mongotest.Fail("not authorized", 13),
	)

	if _, err := repo.UpsertUser(t.Context(), "919876543210", time.Now()); err == nil {
		t.Fatal("UpsertUser() error = nil, want the failure to surface")
	}
}

func TestUpsertUserWrapsErrors(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("findAndModify", mongotest.Fail("not authorized", 13))

	if _, err := repo.UpsertUser(t.Context(), testPhone, time.Now()); err == nil {
		t.Fatal("UpsertUser() error = nil, want the failure to surface")
	}
}

// TestUpsertUserSurfacesAFailedCreate covers the final upsert failing.
func TestUpsertUserSurfacesAFailedCreate(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.RespondSequence("findAndModify",
		mongotest.Reply(noDoc()),
		mongotest.Reply(noDoc()),
		mongotest.Fail("not authorized", 13),
	)

	if _, err := repo.UpsertUser(t.Context(), "919876543210", time.Now()); err == nil {
		t.Fatal("UpsertUser() error = nil, want the failure to surface")
	}
}

func TestUserByIDReturnsTheUser(t *testing.T) {
	repo, fake := newAuthRepo(t)
	id := bson.NewObjectID()
	fake.Respond("find", mongotest.Cursor(usersNS, bson.D{
		{Key: "_id", Value: id},
		{Key: "phone", Value: testPhone},
	}))

	user, err := repo.UserByID(t.Context(), id)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if user.ID != id {
		t.Errorf("id = %v, want %v", user.ID, id)
	}
}

func TestUserByIDReportsNotFound(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("find", mongotest.Cursor(usersNS))

	if _, err := repo.UserByID(t.Context(), bson.NewObjectID()); !errors.Is(err, auth.ErrUserNotFound) {
		t.Errorf("UserByID() error = %v, want ErrUserNotFound", err)
	}
}

func TestUserByIDWrapsErrors(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("find", mongotest.Fail("not authorized", 13))

	_, err := repo.UserByID(t.Context(), bson.NewObjectID())
	if err == nil || errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("UserByID() error = %v, want a wrapped database failure", err)
	}
}

func TestEnsureIndexesCreatesTheCorrectnessConstraints(t *testing.T) {
	repo, fake := newAuthRepo(t)

	if err := repo.EnsureIndexes(t.Context()); err != nil {
		t.Fatalf("EnsureIndexes() error = %v, want nil", err)
	}

	var sawUniquePhone, sawTTL bool
	for _, req := range fake.Requests() {
		if req.Command != "createIndexes" {
			continue
		}
		specs, err := bson.Raw(req.Doc.Lookup("indexes").Array()).Values()
		if err != nil {
			t.Fatalf("reading index specs: %v", err)
		}
		for _, spec := range specs {
			doc := spec.Document()
			if unique, err := doc.LookupErr("unique"); err == nil && unique.Boolean() {
				if _, err := doc.Lookup("key").Document().LookupErr("phone"); err == nil {
					sawUniquePhone = true
				}
			}
			if _, err := doc.LookupErr("expireAfterSeconds"); err == nil {
				sawTTL = true
			}
		}
	}

	// Both are correctness constraints, not optimisations.
	if !sawUniquePhone {
		t.Error("users.phone must be unique, or the sign-in upsert races")
	}
	if !sawTTL {
		t.Error("otp_codes needs a TTL index, or spent codes are never deleted")
	}
}

func TestEnsureIndexesWrapsUserIndexErrors(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("createIndexes", mongotest.Fail("not authorized", 13))

	err := repo.EnsureIndexes(t.Context())
	if err == nil {
		t.Fatal("EnsureIndexes() error = nil, want the failure to surface")
	}
	if !strings.Contains(err.Error(), "ensure users indexes") {
		t.Errorf("error = %q, want it to name the failing step", err)
	}
}

func TestSetAddressesReplacesTheWholeList(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 1}, {Key: "nModified", Value: 1}, {Key: "ok", Value: 1},
	}))

	id := bson.NewObjectID()
	addrs := []auth.Address{{
		ID: bson.NewObjectID(), Name: "Ananya Sharma", Email: "ananya@example.com",
		Line1: "12, Anand Residency, MG Road",
		City:  "Pune", State: "Maharashtra", Pin: "411001", IsDefault: true,
	}}
	if err := repo.SetAddresses(t.Context(), id, addrs, time.Now()); err != nil {
		t.Fatalf("SetAddresses() error = %v, want nil", err)
	}

	req, ok := fake.LastRequest("update")
	if !ok {
		t.Fatal("no update command was sent")
	}
	updates := req.Sequence("updates")
	set := updates[0].Lookup("u").Document().Lookup("$set").Document()

	stored, err := set.LookupErr("addresses")
	if err != nil {
		t.Fatalf("no addresses in the write: %v", set)
	}
	if stored.Type != bson.TypeArray {
		t.Errorf("addresses stored as %v, want an array", stored.Type)
	}
	if _, err := set.LookupErr("updatedAt"); err != nil {
		t.Error("SetAddresses must stamp updatedAt")
	}
	// An address must attach to a real account, never conjure one.
	if upsert, err := updates[0].LookupErr("upsert"); err == nil && upsert.Boolean() {
		t.Error("SetAddresses must not upsert")
	}
}

func TestSetAddressesReportsAMissingUser(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("update", mongotest.Reply(bson.D{
		{Key: "n", Value: 0}, {Key: "nModified", Value: 0}, {Key: "ok", Value: 1},
	}))

	err := repo.SetAddresses(t.Context(), bson.NewObjectID(), nil, time.Now())
	if !errors.Is(err, auth.ErrUserNotFound) {
		t.Errorf("SetAddresses() error = %v, want ErrUserNotFound", err)
	}
}

func TestSetAddressesWrapsErrors(t *testing.T) {
	repo, fake := newAuthRepo(t)
	fake.Respond("update", mongotest.Fail("not authorized", 13))

	err := repo.SetAddresses(t.Context(), bson.NewObjectID(), nil, time.Now())
	if err == nil {
		t.Fatal("SetAddresses() error = nil, want the failure to surface")
	}
	if !strings.Contains(err.Error(), "auth: set addresses") {
		t.Errorf("error = %q, want it wrapped", err)
	}
}
