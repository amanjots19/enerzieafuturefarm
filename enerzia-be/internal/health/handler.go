// Package health serves the liveness endpoint used by deploys and uptime
// checks.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/enerzia/enerzia-be/internal/httpx"
)

// Dependency states reported in the response.
const (
	statusOK       = "ok"
	statusDegraded = "degraded"
	depUp          = "up"
	depDown        = "down"
)

// pingTimeout bounds the dependency check so a hung database cannot hold the
// health endpoint open.
const pingTimeout = 2 * time.Second

// Pinger is the health check a dependency must satisfy. *mongodb.Client
// implements it; tests substitute a stub.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Response is the payload under the "data" key.
type Response struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
	Mongo         string `json:"mongo"`
}

// Handler answers GET /health.
type Handler struct {
	mongo   Pinger
	version string
	started time.Time
	logger  *slog.Logger
	now     func() time.Time // injectable so uptime is deterministic in tests
}

// NewHandler builds the health handler. started is the process start time,
// used to report uptime.
func NewHandler(mongo Pinger, version string, started time.Time, logger *slog.Logger) *Handler {
	return &Handler{
		mongo:   mongo,
		version: version,
		started: started,
		logger:  logger,
		now:     time.Now,
	}
}

// ServeHTTP reports process health. It returns 503 when a dependency is down
// so orchestrators can act on the status code alone, but always writes a body
// describing which dependency failed.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Status:        statusOK,
		Version:       h.version,
		UptimeSeconds: int64(h.now().Sub(h.started).Seconds()),
		Mongo:         depUp,
	}

	ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
	defer cancel()

	if err := h.mongo.Ping(ctx); err != nil {
		// Logged, not returned: the driver error can name hosts and users.
		h.logger.WarnContext(ctx, "health check: mongo unreachable", slog.Any("error", err))
		resp.Status = statusDegraded
		resp.Mongo = depDown
		httpx.WriteData(w, http.StatusServiceUnavailable, resp)
		return
	}

	httpx.WriteData(w, http.StatusOK, resp)
}
