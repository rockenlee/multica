package handler

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/util/secretbox"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TestZentaoStatusOutboundRecordsEvent: outbound status sync to ZenTao now drives
// the real action endpoint (PUT /tasks/{id}/start|finish|…) and records a
// sync_event. With the hybrid gate open and a closed base URL, the action HTTP
// call fails, so an explicit "error" event is written — proving status outbound
// is no longer silent and that success/error/skipped events are recorded.
func TestZentaoStatusOutboundRecordsEvent(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	conn := seedZentaoConnection(t, ctx, "zt-status-event") // base_url http://127.0.0.1:1 (closed)

	var projectID string
	if err := testPool.QueryRow(ctx, `INSERT INTO project (workspace_id, title) VALUES ($1, 'zt-status') RETURNING id`, testWorkspaceID).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM project WHERE id=$1`, projectID) })

	// Hybrid outbound gate: binding.outbound + per-user issue_sync.outbound.
	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_project_binding (workspace_id, project_id, connection_id, external_ref, inbound_enabled_at, outbound_enabled_at)
		 VALUES ($1, $2, $3, '{"execution_id":"447"}'::jsonb, now(), now())`,
		testWorkspaceID, projectID, uuidToString(conn.ID)); err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_issue_sync_setting (workspace_id, user_id, provider, outbound_enabled_at)
		 VALUES ($1, $2, 'zentao', now())
		 ON CONFLICT (workspace_id, user_id, provider) DO UPDATE SET outbound_enabled_at = now()`,
		testWorkspaceID, testUserID); err != nil {
		t.Fatalf("seed sync setting: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_issue_sync_setting WHERE workspace_id=$1 AND user_id=$2 AND provider='zentao'`, testWorkspaceID, testUserID)
	})

	key := make([]byte, secretbox.KeySize)
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatalf("secretbox.New: %v", err)
	}
	sealed, err := box.Seal([]byte(`{"token":"t"}`))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_user_account (workspace_id, connection_id, user_id, account_key, account_name, credential_encrypted, status)
		 VALUES ($1, $2, $3, 'zt-status-acct', 'zt', $4, 'active')`,
		testWorkspaceID, uuidToString(conn.ID), testUserID, sealed); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	prev := testHandler.IntegrationSecrets
	testHandler.IntegrationSecrets = box
	t.Cleanup(func() { testHandler.IntegrationSecrets = prev })

	issue := db.Issue{
		ID:          parseUUID("99999999-2222-3333-4444-555555550006"),
		WorkspaceID: parseUUID(testWorkspaceID),
		ProjectID:   parseUUID(projectID),
		Metadata:    []byte(`{"source_system":"zentao","source_id":"6353"}`),
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_sync_event WHERE workspace_id=$1 AND provider='zentao' AND object_id=$2`, testWorkspaceID, uuidToString(issue.ID))
	})

	// in_review → ZenTao "doing" (start action). Closed base URL → HTTP fails → error event.
	testHandler.pushIssueStatusOutbound(issue, "in_review", parseUUID(testUserID))

	var status string
	if err := testPool.QueryRow(ctx,
		`SELECT status FROM integration_sync_event WHERE workspace_id=$1 AND provider='zentao' AND direction='outbound' AND object_type='issue' AND object_id=$2 ORDER BY occurred_at DESC LIMIT 1`,
		testWorkspaceID, uuidToString(issue.ID)).Scan(&status); err != nil {
		t.Fatalf("no zentao status outbound event recorded (still silent?): %v", err)
	}
	if status != "error" {
		t.Fatalf("status outbound event = %q, want \"error\" (closed base URL → action HTTP fails)", status)
	}
}
