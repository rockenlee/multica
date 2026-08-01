package service

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestRuntimeReadinessAt(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		runtime    db.AgentRuntime
		wantReady  bool
		wantReason string
	}{
		{
			name: "fresh online runtime",
			runtime: db.AgentRuntime{
				Status:     "online",
				LastSeenAt: pgtype.Timestamptz{Time: now.Add(-RuntimeReadinessHeartbeatMaxAge), Valid: true},
			},
			wantReady: true,
		},
		{
			name: "status offline",
			runtime: db.AgentRuntime{
				Status:     "offline",
				LastSeenAt: pgtype.Timestamptz{Time: now, Valid: true},
			},
			wantReason: "agent runtime is offline",
		},
		{
			name: "missing heartbeat",
			runtime: db.AgentRuntime{
				Status: "online",
			},
			wantReason: "agent runtime heartbeat is missing",
		},
		{
			name: "stale heartbeat despite online status",
			runtime: db.AgentRuntime{
				Status:     "online",
				LastSeenAt: pgtype.Timestamptz{Time: now.Add(-RuntimeReadinessHeartbeatMaxAge - time.Nanosecond), Valid: true},
			},
			wantReason: "agent runtime heartbeat is stale",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ready, reason, err := runtimeReadinessAt(now, test.runtime)
			if err != nil {
				t.Fatalf("runtimeReadinessAt: %v", err)
			}
			if ready != test.wantReady {
				t.Fatalf("ready = %v, want %v", ready, test.wantReady)
			}
			if reason != test.wantReason {
				t.Fatalf("reason = %q, want %q", reason, test.wantReason)
			}
		})
	}
}
