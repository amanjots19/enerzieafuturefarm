package mongodb_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/enerzia/enerzia-be/internal/mongodb"
	"github.com/enerzia/enerzia-be/internal/mongotest"
)

const testDB = "enerzia_test"

// connectToFake starts a fake server and connects to it, failing the test if
// the handshake does not complete.
func connectToFake(t *testing.T) (*mongodb.Client, *mongotest.Server) {
	t.Helper()

	fake := mongotest.Start(t)
	client, err := mongodb.Connect(t.Context(), fake.URI(), testDB, 5*time.Second)
	if err != nil {
		t.Fatalf("Connect() error = %v, want nil (commands seen: %v)", err, fake.Commands())
	}
	t.Cleanup(func() {
		if err := client.Disconnect(t.Context()); err != nil {
			t.Errorf("Disconnect() error = %v", err)
		}
	})
	return client, fake
}

func TestConnectRejectsEmptyArguments(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		db      string
		wantErr error
	}{
		{name: "empty uri", uri: "mongodb://localhost:27017", db: "", wantErr: mongodb.ErrEmptyDatabase},
		{name: "empty database", uri: "", db: testDB, wantErr: mongodb.ErrEmptyURI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := mongodb.Connect(t.Context(), tt.uri, tt.db, time.Second)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Connect() error = %v, want %v", err, tt.wantErr)
			}
			if client != nil {
				t.Error("Connect() must return a nil client alongside an error")
			}
		})
	}
}

func TestConnectRejectsMalformedURI(t *testing.T) {
	client, err := mongodb.Connect(t.Context(), "not-a-mongo-uri", testDB, time.Second)
	if err == nil {
		t.Fatal("Connect() error = nil, want a parse failure")
	}
	if client != nil {
		t.Error("Connect() must return a nil client alongside an error")
	}
	if !strings.Contains(err.Error(), "mongodb: connect") {
		t.Errorf("error = %q, want it wrapped with 'mongodb: connect'", err)
	}
}

func TestConnectFailsWhenNothingIsListening(t *testing.T) {
	// Start a fake only to borrow its address, then close it, so the port is
	// certain to be free and refusing connections.
	fake := mongotest.Start(t)
	uri := fake.URI()
	fake.Close()

	client, err := mongodb.Connect(t.Context(), uri, testDB, 500*time.Millisecond)
	if err == nil {
		_ = client.Disconnect(t.Context())
		t.Fatal("Connect() error = nil, want a ping failure")
	}
	if !strings.Contains(err.Error(), "mongodb: ping") {
		t.Errorf("error = %q, want it wrapped with 'mongodb: ping'", err)
	}
}

func TestConnectFailsWhenHandshakeIsRefused(t *testing.T) {
	fake := mongotest.Start(t)
	failure := mongotest.Fail("not authorized on admin", 13)
	for _, cmd := range []string{"isMaster", "ismaster", "hello"} {
		fake.Respond(cmd, failure)
	}

	client, err := mongodb.Connect(t.Context(), fake.URI(), testDB, time.Second)
	if err == nil {
		_ = client.Disconnect(t.Context())
		t.Fatal("Connect() error = nil, want the refused handshake to surface")
	}
	if !strings.Contains(err.Error(), "mongodb: ping") {
		t.Errorf("error = %q, want it wrapped with 'mongodb: ping'", err)
	}
}

func TestConnectErrorNeverLeaksCredentials(t *testing.T) {
	fake := mongotest.Start(t)
	uri := fake.URI()
	fake.Close()

	// Same address, now with credentials in the URI.
	withCreds := strings.Replace(uri, "mongodb://", "mongodb://appuser:hunter2@", 1)

	_, err := mongodb.Connect(t.Context(), withCreds, testDB, 500*time.Millisecond)
	if err == nil {
		t.Fatal("Connect() error = nil, want a failure")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("connection error leaks the password: %q", err)
	}
}

func TestConnectSucceedsAgainstFakeServer(t *testing.T) {
	client, fake := connectToFake(t)

	if client.DB() == nil {
		t.Fatal("DB() = nil, want a database handle")
	}
	if got := client.DB().Name(); got != testDB {
		t.Errorf("DB().Name() = %q, want %q", got, testDB)
	}
	// Connect must verify the connection, not just construct a client.
	if !slices.Contains(fake.Commands(), "ping") {
		t.Errorf("Connect() never pinged; commands seen: %v", fake.Commands())
	}
}

func TestPingSucceedsWhenServerIsHealthy(t *testing.T) {
	client, fake := connectToFake(t)

	before := len(fake.Commands())
	if err := client.Ping(t.Context()); err != nil {
		t.Fatalf("Ping() error = %v, want nil", err)
	}
	if len(fake.Commands()) <= before {
		t.Error("Ping() did not reach the server")
	}
}

func TestPingFailsWhenServerRejectsTheCommand(t *testing.T) {
	client, fake := connectToFake(t)

	fake.Respond("ping", mongotest.Fail("interrupted at shutdown", 11600))

	err := client.Ping(t.Context())
	if err == nil {
		t.Fatal("Ping() error = nil, want the server error to surface")
	}
	if !strings.Contains(err.Error(), "mongodb: ping") {
		t.Errorf("error = %q, want it wrapped with 'mongodb: ping'", err)
	}
}

func TestPingRecoversAfterFailureIsCleared(t *testing.T) {
	client, fake := connectToFake(t)

	fake.Respond("ping", mongotest.Fail("transient", 6))
	if err := client.Ping(t.Context()); err == nil {
		t.Fatal("Ping() error = nil while the command was failing")
	}

	fake.ClearCommand("ping")
	if err := client.Ping(t.Context()); err != nil {
		t.Errorf("Ping() error = %v after the failure was cleared, want nil", err)
	}
}

func TestPingFailsAfterDisconnect(t *testing.T) {
	fake := mongotest.Start(t)
	client, err := mongodb.Connect(t.Context(), fake.URI(), testDB, 5*time.Second)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	if err := client.Disconnect(t.Context()); err != nil {
		t.Fatalf("Disconnect() error = %v, want nil", err)
	}
	if err := client.Ping(t.Context()); err == nil {
		t.Error("Ping() error = nil after Disconnect, want an error")
	}
}

func TestDisconnectIsIdempotent(t *testing.T) {
	fake := mongotest.Start(t)
	client, err := mongodb.Connect(t.Context(), fake.URI(), testDB, 5*time.Second)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	if err := client.Disconnect(t.Context()); err != nil {
		t.Fatalf("first Disconnect() error = %v, want nil", err)
	}
	if err := client.Disconnect(t.Context()); err != nil {
		t.Errorf("second Disconnect() error = %v, want nil", err)
	}
}

func TestNilClientIsSafe(t *testing.T) {
	// Shutdown paths call these without knowing whether Connect succeeded.
	var client *mongodb.Client

	if err := client.Disconnect(t.Context()); err != nil {
		t.Errorf("Disconnect() on nil client = %v, want nil", err)
	}
	if err := client.Ping(t.Context()); err == nil {
		t.Error("Ping() on nil client = nil, want an error")
	}
}
