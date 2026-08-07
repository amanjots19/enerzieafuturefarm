// Package mongodb owns the MongoDB connection. Nothing above the repository
// layer should import the driver.
package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// ErrEmptyURI is returned when Connect is called without a connection string.
var ErrEmptyURI = errors.New("mongodb: uri is required")

// ErrEmptyDatabase is returned when Connect is called without a database name.
var ErrEmptyDatabase = errors.New("mongodb: database name is required")

// Client wraps the driver client together with the single database this
// service uses.
type Client struct {
	client *mongo.Client
	db     *mongo.Database
}

// Connect dials MongoDB and verifies the connection with a ping, so a bad URI
// or unreachable cluster fails at startup instead of on the first request.
// timeout bounds both the dial and the ping.
func Connect(ctx context.Context, uri, database string, timeout time.Duration) (*Client, error) {
	if uri == "" {
		return nil, ErrEmptyURI
	}
	if database == "" {
		return nil, ErrEmptyDatabase
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetTimeout(timeout))
	if err != nil {
		// Wrapped without the URI, which carries the password.
		return nil, fmt.Errorf("mongodb: connect: %w", err)
	}

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("mongodb: ping: %w", err)
	}

	return &Client{client: client, db: client.Database(database)}, nil
}

// DB returns the handle repositories run their queries against.
func (c *Client) DB() *mongo.Database { return c.db }

// Ping reports whether the cluster is currently reachable. It backs the
// "mongo" field of the health endpoint.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("mongodb: not connected")
	}
	if err := c.client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("mongodb: ping: %w", err)
	}
	return nil
}

// Disconnect closes the pool. It is idempotent and safe to call on a nil or
// partially constructed Client, so shutdown paths — where a deferred close and
// an explicit one can both fire — need no guards. Closing an already-closed
// client is success, not an error worth logging.
func (c *Client) Disconnect(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}
	if err := c.client.Disconnect(ctx); err != nil && !errors.Is(err, mongo.ErrClientDisconnected) {
		return fmt.Errorf("mongodb: disconnect: %w", err)
	}
	return nil
}
