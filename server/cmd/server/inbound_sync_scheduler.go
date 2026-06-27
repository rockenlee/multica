package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/multica-ai/multica/server/internal/handler"
)

// inboundSyncInterval is how often the worker polls external systems for new
// issues/tasks to mirror inbound. Short enough to feel responsive, long enough
// to be gentle on external APIs.
const inboundSyncInterval = 2 * time.Minute

// runInboundSyncScheduler periodically pulls external issues/tasks from every
// inbound-enabled integration target and mirrors them into Multica. Runs as a
// background goroutine; exits when ctx is cancelled (mirrors runAutopilotScheduler).
func runInboundSyncScheduler(ctx context.Context, h *handler.Handler) {
	ticker := time.NewTicker(inboundSyncInterval)
	defer ticker.Stop()

	// Kick once at startup so a freshly-enabled connection syncs without
	// waiting a full interval.
	tickInboundSync(ctx, h)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickInboundSync(ctx, h)
		}
	}
}

func tickInboundSync(ctx context.Context, h *handler.Handler) {
	targets, err := h.ListInboundSyncTargets(ctx)
	if err != nil {
		slog.Warn("inbound sync: failed to list targets", "error", err)
		return
	}
	imported := 0
	for _, t := range targets {
		// Error isolation: one provider/target failing must not block others.
		n, err := h.SyncInboundTarget(ctx, t)
		if err != nil {
			slog.Warn("inbound sync: target failed", "provider", t.Provider, "error", err)
			continue
		}
		imported += n
	}
	if imported > 0 {
		slog.Info("inbound sync: mirrored external items", "imported", imported, "targets", len(targets))
	}
}
