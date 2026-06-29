package handler

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/util/secretbox"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// seedGitlabOutbound opens the hybrid outbound gate for a fresh GitLab connection
// (binding + per-user setting + sealed credential) and returns the connection id,
// project id, and the secretbox used to seal the credential. The connection's
// base_url is closed (http://127.0.0.1:1) so any real GitLab call fails.
func seedGitlabOutbound(t *testing.T, ctx context.Context, name string) (connID, projectID string, box *secretbox.Box) {
	t.Helper()
	if err := testPool.QueryRow(ctx,
		`INSERT INTO integration_connection (workspace_id, provider, name, base_url, status, sync_enabled_at)
		 VALUES ($1, 'gitlab', $2, 'http://127.0.0.1:1', 'active', now()) RETURNING id`,
		testWorkspaceID, name).Scan(&connID); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_connection WHERE id=$1`, connID)
	})

	if err := testPool.QueryRow(ctx, `INSERT INTO project (workspace_id, title) VALUES ($1, $2) RETURNING id`, testWorkspaceID, name).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM project WHERE id=$1`, projectID) })

	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_project_binding (workspace_id, project_id, connection_id, external_ref, inbound_enabled_at, outbound_enabled_at)
		 VALUES ($1, $2, $3, '{"project":"group/proj"}'::jsonb, now(), now())`,
		testWorkspaceID, projectID, connID); err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_issue_sync_setting (workspace_id, user_id, provider, outbound_enabled_at)
		 VALUES ($1, $2, 'gitlab', now())
		 ON CONFLICT (workspace_id, user_id, provider) DO UPDATE SET outbound_enabled_at = now()`,
		testWorkspaceID, testUserID); err != nil {
		t.Fatalf("seed sync setting: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_issue_sync_setting WHERE workspace_id=$1 AND user_id=$2 AND provider='gitlab'`, testWorkspaceID, testUserID)
	})

	key := make([]byte, secretbox.KeySize)
	b, err := secretbox.New(key)
	if err != nil {
		t.Fatalf("secretbox.New: %v", err)
	}
	sealed, err := b.Seal([]byte("glpat-test"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_user_account (workspace_id, connection_id, user_id, account_key, account_name, credential_encrypted, status)
		 VALUES ($1, $2, $3, 'gl-acct', 'gl', $4, 'active')`,
		testWorkspaceID, connID, testUserID, sealed); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return connID, projectID, b
}

// TestGitlabStatusOutboundRecordsEvent: with the hybrid gate open and a closed
// base URL, the GitLab UpdateIssueState call fails, so an "error" event is
// recorded against the gate-resolved connection — proving status outbound is
// auditable and attributed to the right connection.
func TestGitlabStatusOutboundRecordsEvent(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	connID, projectID, box := seedGitlabOutbound(t, ctx, "gl-status-event")

	prev := testHandler.IntegrationSecrets
	testHandler.IntegrationSecrets = box
	t.Cleanup(func() { testHandler.IntegrationSecrets = prev })

	issue := db.Issue{
		ID:          parseUUID("99999999-2222-3333-4444-5555555500a1"),
		WorkspaceID: parseUUID(testWorkspaceID),
		ProjectID:   parseUUID(projectID),
		Metadata:    []byte(`{"source_system":"gitlab","source_id":"42"}`),
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_sync_event WHERE workspace_id=$1 AND provider='gitlab' AND object_id=$2`, testWorkspaceID, uuidToString(issue.ID))
	})

	testHandler.pushIssueStatusOutbound(issue, "done", parseUUID(testUserID))

	var status, eventConnID, objectType, externalID string
	if err := testPool.QueryRow(ctx,
		`SELECT status, connection_id, object_type, external_id FROM integration_sync_event
		 WHERE workspace_id=$1 AND provider='gitlab' AND direction='outbound' AND object_id=$2 ORDER BY occurred_at DESC LIMIT 1`,
		testWorkspaceID, uuidToString(issue.ID)).Scan(&status, &eventConnID, &objectType, &externalID); err != nil {
		t.Fatalf("no gitlab status outbound event recorded (still silent?): %v", err)
	}
	if status != "error" {
		t.Fatalf("status outbound event = %q, want \"error\" (closed base URL → call fails)", status)
	}
	if eventConnID != connID {
		t.Fatalf("event connection_id = %q, want gate-resolved %q", eventConnID, connID)
	}
	if objectType != "issue" {
		t.Fatalf("object_type = %q, want \"issue\"", objectType)
	}
	if externalID != "42" {
		t.Fatalf("external_id = %q, want \"42\"", externalID)
	}
}

// TestGitlabCommentOutboundRecordsEvent: comment outbound to GitLab fails against
// the closed base URL and records an "error" comment event attributed to the
// gate-resolved connection.
func TestGitlabCommentOutboundRecordsEvent(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	connID, projectID, box := seedGitlabOutbound(t, ctx, "gl-comment-event")

	prev := testHandler.IntegrationSecrets
	testHandler.IntegrationSecrets = box
	t.Cleanup(func() { testHandler.IntegrationSecrets = prev })

	issue := db.Issue{
		ID:          parseUUID("99999999-2222-3333-4444-5555555500a2"),
		WorkspaceID: parseUUID(testWorkspaceID),
		ProjectID:   parseUUID(projectID),
		Metadata:    []byte(`{"source_system":"gitlab","source_id":"42"}`),
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_sync_event WHERE workspace_id=$1 AND provider='gitlab' AND object_id=$2`, testWorkspaceID, uuidToString(issue.ID))
	})

	testHandler.pushIssueCommentOutbound(issue, "hello from multica", parseUUID(testUserID))

	var status, eventConnID, objectType, externalID string
	if err := testPool.QueryRow(ctx,
		`SELECT status, connection_id, object_type, external_id FROM integration_sync_event
		 WHERE workspace_id=$1 AND provider='gitlab' AND direction='outbound' AND object_id=$2 ORDER BY occurred_at DESC LIMIT 1`,
		testWorkspaceID, uuidToString(issue.ID)).Scan(&status, &eventConnID, &objectType, &externalID); err != nil {
		t.Fatalf("no gitlab comment outbound event recorded: %v", err)
	}
	if status != "error" {
		t.Fatalf("comment outbound event = %q, want \"error\" (closed base URL → call fails)", status)
	}
	if eventConnID != connID {
		t.Fatalf("event connection_id = %q, want gate-resolved %q", eventConnID, connID)
	}
	if objectType != "comment" {
		t.Fatalf("object_type = %q, want \"comment\"", objectType)
	}
	if externalID != "42" {
		t.Fatalf("external_id = %q, want \"42\"", externalID)
	}
}
