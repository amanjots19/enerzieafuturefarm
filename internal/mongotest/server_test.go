package mongotest_test

import (
	"encoding/binary"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/enerzia/enerzia-be/internal/mongotest"
)

// A broken fake would make every test that depends on it pass for the wrong
// reason, so its own behaviour is asserted here with the real driver.

func connect(t *testing.T, uri string) *mongo.Client {
	t.Helper()
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetTimeout(5 * time.Second))
	if err != nil {
		t.Fatalf("driver Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(t.Context()) })
	return client
}

func TestURIIsLoopbackDirectConnection(t *testing.T) {
	s := mongotest.Start(t)

	uri := s.URI()
	if !strings.HasPrefix(uri, "mongodb://127.0.0.1:") {
		t.Errorf("URI() = %q, want a loopback address", uri)
	}
	if !strings.Contains(uri, "directConnection=true") {
		t.Errorf("URI() = %q, want directConnection=true so the driver skips discovery", uri)
	}
}

func TestCompletesDriverHandshakeAndPing(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())

	if err := client.Ping(t.Context(), nil); err != nil {
		t.Fatalf("Ping() error = %v, want nil (commands seen: %v)", err, s.Commands())
	}
	if got := s.Commands(); !slices.Contains(got, "ping") {
		t.Errorf("commands = %v, want them to include ping", got)
	}
}

func TestRecordsCommandsInOrder(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())
	if err := client.Ping(t.Context(), nil); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	got := s.Commands()
	if len(got) == 0 {
		t.Fatal("Commands() is empty; nothing was recorded")
	}
	// The handshake always precedes any real command.
	if got[0] != "isMaster" && got[0] != "hello" {
		t.Errorf("first command = %q, want the handshake", got[0])
	}
}

func TestCommandsReturnsACopy(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())
	_ = client.Ping(t.Context(), nil)

	got := s.Commands()
	if len(got) == 0 {
		t.Fatal("no commands recorded")
	}
	got[0] = "mutated"

	if s.Commands()[0] == "mutated" {
		t.Error("Commands() exposed its backing array; callers can corrupt the log")
	}
}

func TestRespondWithFailMakesTheCommandFail(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())

	if err := client.Ping(t.Context(), nil); err != nil {
		t.Fatalf("Ping() error = %v before any failure was configured", err)
	}

	s.Respond("ping", mongotest.Fail("shutting down", 11600))

	err := client.Ping(t.Context(), nil)
	if err == nil {
		t.Fatal("Ping() error = nil, want the configured failure")
	}
	if !strings.Contains(err.Error(), "shutting down") {
		t.Errorf("error = %q, want it to carry the configured message", err)
	}
}

func TestFailDefaultsTheErrorCode(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())

	s.Respond("ping", mongotest.Fail("no code given", 0))

	if err := client.Ping(t.Context(), nil); err == nil {
		t.Fatal("Ping() error = nil, want a failure even without an explicit code")
	}
}

func TestClearCommandRestoresSuccess(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())

	s.Respond("ping", mongotest.Fail("temporary", 0))
	if err := client.Ping(t.Context(), nil); err == nil {
		t.Fatal("Ping() error = nil while failing")
	}

	s.ClearCommand("ping")
	if err := client.Ping(t.Context(), nil); err != nil {
		t.Errorf("Ping() error = %v after ClearCommand, want nil", err)
	}
}

func TestCustomReplyDocumentIsReturned(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())

	s.Respond("buildInfo", mongotest.CommandResult{
		Doc: bson.D{{Key: "ok", Value: 1}, {Key: "version", Value: "8.0.0"}},
	})

	var out bson.M
	err := client.Database("admin").
		RunCommand(t.Context(), bson.D{{Key: "buildInfo", Value: 1}}).
		Decode(&out)
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if out["version"] != "8.0.0" {
		t.Errorf("version = %v, want 8.0.0 from the configured reply", out["version"])
	}
}

func TestUnknownCommandsSucceedByDefault(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())

	err := client.Database("admin").
		RunCommand(t.Context(), bson.D{{Key: "somethingNew", Value: 1}}).
		Err()
	if err != nil {
		t.Errorf("RunCommand() error = %v, want a default ok:1 reply", err)
	}
}

func TestRequestsRecordTheCommandDocument(t *testing.T) {
	// Repository tests assert on the query that was built, not just its name.
	s := mongotest.Start(t)
	client := connect(t, s.URI())

	s.Respond("find", mongotest.Cursor("enerzia.products",
		bson.D{{Key: "_id", Value: "powder"}},
	))

	cur, err := client.Database("enerzia").Collection("products").
		Find(t.Context(), bson.D{{Key: "form", Value: "Powder"}})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	defer func() { _ = cur.Close(t.Context()) }()

	req, ok := s.LastRequest("find")
	if !ok {
		t.Fatal("LastRequest(find) found nothing")
	}
	if got := req.Doc.Lookup("find").StringValue(); got != "products" {
		t.Errorf("find collection = %q, want products", got)
	}
	filter, err := req.Doc.LookupErr("filter")
	if err != nil {
		t.Fatalf("no filter in the recorded command: %v", err)
	}
	if got := filter.Document().Lookup("form").StringValue(); got != "Powder" {
		t.Errorf("filter.form = %q, want Powder", got)
	}
}

func TestCursorRepliesAreDecodable(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())

	s.Respond("find", mongotest.Cursor("enerzia.products",
		bson.D{{Key: "_id", Value: "powder"}, {Key: "name", Value: "Pure Spirulina Powder"}},
		bson.D{{Key: "_id", Value: "tablets"}, {Key: "name", Value: "Spirulina Tablets 500 mg"}},
	))

	cur, err := client.Database("enerzia").Collection("products").Find(t.Context(), bson.D{})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}

	var out []struct {
		ID   string `bson:"_id"`
		Name string `bson:"name"`
	}
	if err := cur.All(t.Context(), &out); err != nil {
		t.Fatalf("cursor.All() error = %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("decoded %d documents, want 2", len(out))
	}
	if out[0].ID != "powder" || out[1].Name != "Spirulina Tablets 500 mg" {
		t.Errorf("decoded %+v, want the documents supplied to Cursor", out)
	}
}

func TestEmptyCursorDecodesToNothing(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())

	s.Respond("find", mongotest.Cursor("enerzia.products"))

	cur, err := client.Database("enerzia").Collection("products").Find(t.Context(), bson.D{})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}

	var out []bson.M
	if err := cur.All(t.Context(), &out); err != nil {
		t.Fatalf("cursor.All() error = %v", err)
	}
	if len(out) != 0 {
		t.Errorf("decoded %d documents, want 0", len(out))
	}
}

func TestRequestsReturnsACopy(t *testing.T) {
	s := mongotest.Start(t)
	client := connect(t, s.URI())
	_ = client.Ping(t.Context(), nil)

	got := s.Requests()
	if len(got) == 0 {
		t.Fatal("no requests recorded")
	}
	got[0].Command = "mutated"

	if s.Requests()[0].Command == "mutated" {
		t.Error("Requests() exposed its backing array")
	}
}

func TestLastRequestReportsMissingCommands(t *testing.T) {
	s := mongotest.Start(t)
	if _, ok := s.LastRequest("aggregate"); ok {
		t.Error("LastRequest() ok = true for a command that never arrived")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	s := mongotest.Start(t)
	s.Close()
	s.Close() // the tb.Cleanup call makes a third; none may panic or hang
}

func TestCloseStopsAcceptingConnections(t *testing.T) {
	s := mongotest.Start(t)
	addr := strings.TrimSuffix(strings.TrimPrefix(s.URI(), "mongodb://"), "/?directConnection=true")
	s.Close()

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Error("the listener still accepts connections after Close()")
	}
}

func TestMalformedMessageClosesTheConnection(t *testing.T) {
	// A garbage header must drop the connection rather than hang or panic.
	s := mongotest.Start(t)
	addr := strings.TrimSuffix(strings.TrimPrefix(s.URI(), "mongodb://"), "/?directConnection=true")

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Declare a 20-byte message whose opcode is not one we serve.
	header := make([]byte, 16)
	binary.LittleEndian.PutUint32(header[0:4], 20)   // messageLength
	binary.LittleEndian.PutUint32(header[4:8], 1)    // requestID
	binary.LittleEndian.PutUint32(header[8:12], 0)   // responseTo
	binary.LittleEndian.PutUint32(header[12:16], 42) // unsupported opCode
	if _, err := conn.Write(append(header, 0, 0, 0, 0)); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Error("server replied to an unsupported opcode; it should close the connection")
	}
}

func TestShortHeaderClosesTheConnection(t *testing.T) {
	s := mongotest.Start(t)
	addr := strings.TrimSuffix(strings.TrimPrefix(s.URI(), "mongodb://"), "/?directConnection=true")

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// messageLength smaller than the 16-byte header is nonsense.
	header := make([]byte, 16)
	binary.LittleEndian.PutUint32(header[0:4], 4)
	binary.LittleEndian.PutUint32(header[12:16], 2013)
	if _, err := conn.Write(header); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Error("server replied to a malformed header; it should close the connection")
	}
}
