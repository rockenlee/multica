package handler

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// seedZentaoConnection inserts one active ZenTao connection and returns it.
func seedZentaoConnection(t *testing.T, ctx context.Context, name string) db.IntegrationConnection {
	t.Helper()
	var connID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO integration_connection (workspace_id, provider, name, base_url, status, sync_enabled_at)
		 VALUES ($1, 'zentao', $2, 'http://127.0.0.1:1', 'active', now()) RETURNING id`,
		testWorkspaceID, name).Scan(&connID); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_connection WHERE id = $1`, connID)
	})
	conn, err := testHandler.Queries.GetIntegrationConnectionInWorkspace(ctx, db.GetIntegrationConnectionInWorkspaceParams{
		ID:          parseUUID(connID),
		WorkspaceID: parseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("load connection: %v", err)
	}
	return conn
}

// TestInboundUpsertSkipsTombstonedItem covers requirement 2: a tombstoned
// external item is not resurrected on the next inbound poll, and a skipped
// inbound sync_event is recorded.
func TestInboundUpsertSkipsTombstonedItem(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	conn := seedZentaoConnection(t, ctx, "tomb-inbound-skip")
	var projectID string
	if err := testPool.QueryRow(ctx, `INSERT INTO project (workspace_id, title) VALUES ($1, 'tomb-inbound-skip') RETURNING id`, testWorkspaceID).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM project WHERE id=$1`, projectID) })

	const sourceID = "TOMB-INB-9001"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_issue_tombstone WHERE workspace_id=$1 AND provider='zentao' AND source_id=$2`, testWorkspaceID, sourceID)
		testPool.Exec(context.Background(), `DELETE FROM integration_sync_event WHERE workspace_id=$1 AND provider='zentao' AND object_id=$2`, testWorkspaceID, sourceID)
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id=$1 AND metadata->>'source_id'=$2`, testWorkspaceID, sourceID)
	})

	// Pre-existing tombstone (user deleted this mirror earlier).
	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_issue_tombstone (workspace_id, provider, source_id) VALUES ($1, 'zentao', $2)`,
		testWorkspaceID, sourceID); err != nil {
		t.Fatalf("seed tombstone: %v", err)
	}

	desc := "should not be created"
	issue, status, _, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, parseUUID(projectID), pgtype.UUID{},
		"Tombstoned task", &desc, "task", sourceID, "https://zentao/task-9001", "open")
	if err != nil {
		t.Fatalf("upsert returned error: %v", err)
	}
	if status != "skipped" {
		t.Fatalf("status = %q, want skipped", status)
	}
	if issue.ID.Valid {
		t.Fatalf("expected no issue to be created, got id %s", uuidToString(issue.ID))
	}

	// No mirror issue exists.
	_, err = testHandler.Queries.GetIssueByExternalSource(ctx, db.GetIssueByExternalSourceParams{
		WorkspaceID: parseUUID(testWorkspaceID), SourceSystem: "zentao", SourceID: sourceID,
	})
	if err == nil {
		t.Fatalf("a mirror issue was resurrected despite the tombstone")
	}

	// A skipped inbound event was recorded.
	var skipped int
	if e := testPool.QueryRow(ctx,
		`SELECT count(*) FROM integration_sync_event WHERE workspace_id=$1 AND provider='zentao' AND direction='inbound' AND status='skipped' AND object_id=$2`,
		testWorkspaceID, sourceID).Scan(&skipped); e != nil {
		t.Fatalf("count skipped events: %v", e)
	}
	if skipped == 0 {
		t.Fatalf("expected a skipped inbound sync_event for the suppressed item")
	}
}

// TestDeleteMirrorWritesTombstone covers requirement 3: deleting a mirror issue
// writes a tombstone unconditionally (even with outbound gates off), so inbound
// can't resurrect it.
func TestDeleteMirrorWritesTombstone(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	const sourceID = "TOMB-DEL-9002"
	projectID := parseUUID("11111111-2222-3333-4444-555555550002")
	issueID := parseUUID("99999999-2222-3333-4444-555555550002")
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_issue_tombstone WHERE workspace_id=$1 AND provider='zentao' AND source_id=$2`, testWorkspaceID, sourceID)
	})

	issue := db.Issue{
		ID:          issueID,
		WorkspaceID: parseUUID(testWorkspaceID),
		ProjectID:   projectID,
		Metadata:    []byte(`{"source_system":"zentao","source_id":"` + sourceID + `","source_url":"https://zentao/task-9002"}`),
	}
	// No binding/gate configured → tombstone must still be written (ungated).
	testHandler.handleMirrorIssueDeleted(issue, parseUUID(testUserID))

	var got struct {
		issueID, srcURL string
	}
	if err := testPool.QueryRow(ctx,
		`SELECT coalesce(issue_id::text,''), coalesce(source_url,'') FROM integration_issue_tombstone WHERE workspace_id=$1 AND provider='zentao' AND source_id=$2`,
		testWorkspaceID, sourceID).Scan(&got.issueID, &got.srcURL); err != nil {
		t.Fatalf("tombstone not written: %v", err)
	}
	if got.issueID != uuidToString(issueID) {
		t.Fatalf("tombstone issue_id = %q, want %s", got.issueID, uuidToString(issueID))
	}
	if got.srcURL != "https://zentao/task-9002" {
		t.Fatalf("tombstone source_url = %q", got.srcURL)
	}
}

// TestDeleteNativeIssueNoTombstone: a native (non-mirror) issue delete must not
// create a tombstone.
func TestDeleteNativeIssueNoTombstone(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	issue := db.Issue{
		ID:          parseUUID("99999999-2222-3333-4444-5555555500a3"),
		WorkspaceID: parseUUID(testWorkspaceID),
		Metadata:    []byte(`{}`),
	}
	testHandler.handleMirrorIssueDeleted(issue, parseUUID(testUserID))

	var n int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM integration_issue_tombstone WHERE workspace_id=$1 AND issue_id=$2`,
		testWorkspaceID, issue.ID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("native issue delete created %d tombstone(s), want 0", n)
	}
}

// TestZentaoCommentOutboundRecordsExplicitFailure: ZenTao v1 REST has no comment
// endpoint, so a comment on a ZenTao mirror CANNOT be delivered. The product
// must surface this as an explicit FAILURE (status=error) — not a silent no-op,
// and not a warning treated as a pass. This test asserts the failure is visible.
func TestZentaoCommentOutboundRecordsExplicitFailure(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	conn := seedZentaoConnection(t, ctx, "tomb-comment-event")

	// pushIssueCommentOutbound early-returns when IntegrationSecrets is nil.
	key := make([]byte, secretbox.KeySize)
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatalf("secretbox.New: %v", err)
	}
	prev := testHandler.IntegrationSecrets
	testHandler.IntegrationSecrets = box
	t.Cleanup(func() { testHandler.IntegrationSecrets = prev })

	var projectID string
	if err := testPool.QueryRow(ctx, `INSERT INTO project (workspace_id, title) VALUES ($1, 'tomb-comment') RETURNING id`, testWorkspaceID).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM project WHERE id=$1`, projectID) })

	issue := db.Issue{
		ID:          parseUUID("99999999-2222-3333-4444-555555550004"),
		WorkspaceID: parseUUID(testWorkspaceID),
		ProjectID:   parseUUID(projectID),
		Metadata:    []byte(`{"source_system":"zentao","source_id":"TOMB-CMT-9004"}`),
	}
	_ = conn
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_sync_event WHERE workspace_id=$1 AND provider='zentao' AND object_id=$2`, testWorkspaceID, uuidToString(issue.ID))
	})
	testHandler.pushIssueCommentOutbound(issue, "评价了，看你能不能出站？", parseUUID(testUserID))

	// The event must exist AND be an explicit error — never success/warning-as-pass.
	var status string
	if err := testPool.QueryRow(ctx,
		`SELECT status FROM integration_sync_event WHERE workspace_id=$1 AND provider='zentao' AND direction='outbound' AND object_type='comment' AND object_id=$2 ORDER BY occurred_at DESC LIMIT 1`,
		testWorkspaceID, uuidToString(issue.ID)).Scan(&status); err != nil {
		t.Fatalf("no zentao comment outbound event recorded (still silent?): %v", err)
	}
	if status != "error" {
		t.Fatalf("zentao comment outbound status = %q, want \"error\" (must be a visible failure, not warning/success)", status)
	}
}

// TestDeleteMirrorTombstoneWithInvalidActor covers blocker 2: even when the
// actor id is invalid (agent/system/non-UUID token), deleting a mirror still
// writes a tombstone (deleted_by NULL) so inbound can't resurrect it — only the
// per-user external cancel is skipped.
func TestDeleteMirrorTombstoneWithInvalidActor(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	const sourceID = "TOMB-DEL-NOACTOR-9005"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_issue_tombstone WHERE workspace_id=$1 AND provider='zentao' AND source_id=$2`, testWorkspaceID, sourceID)
	})
	issue := db.Issue{
		ID:          parseUUID("99999999-2222-3333-4444-555555550005"),
		WorkspaceID: parseUUID(testWorkspaceID),
		Metadata:    []byte(`{"source_system":"zentao","source_id":"` + sourceID + `"}`),
	}
	invalidActor, _ := parseUUIDLoose("not-a-uuid")
	if invalidActor.Valid {
		t.Fatalf("test setup: expected an invalid actor uuid")
	}
	testHandler.handleMirrorIssueDeleted(issue, invalidActor)

	var deletedByNull bool
	if err := testPool.QueryRow(ctx,
		`SELECT deleted_by IS NULL FROM integration_issue_tombstone WHERE workspace_id=$1 AND provider='zentao' AND source_id=$2`,
		testWorkspaceID, sourceID).Scan(&deletedByNull); err != nil {
		t.Fatalf("tombstone NOT written for invalid-actor delete (resurrection risk): %v", err)
	}
	if !deletedByNull {
		t.Fatalf("expected deleted_by NULL when actor is invalid")
	}
}

// TestAutoBindingEnablesOutbound covers requirement 5: a ZenTao execution
// auto-binding defaults outbound + issue_sync ON (not inbound-only).
func TestAutoBindingEnablesOutbound(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	// syncZentaoExecutionBinding requires exactly one active zentao connection.
	testPool.Exec(ctx, `DELETE FROM integration_connection WHERE workspace_id=$1 AND provider='zentao'`, testWorkspaceID)
	conn := seedZentaoConnection(t, ctx, "tomb-autobind-outbound")

	var projectID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO project (workspace_id, title) VALUES ($1, 'autobind outbound') RETURNING id`,
		testWorkspaceID).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM project WHERE id=$1`, projectID) })

	testHandler.syncZentaoExecutionBinding(ctx,
		db.Project{ID: parseUUID(projectID), WorkspaceID: parseUUID(testWorkspaceID)},
		[]byte(`{"execution_id":"447"}`), nil, parseUUID(testUserID))

	var inbound, outbound, issueSync bool
	if err := testPool.QueryRow(ctx,
		`SELECT inbound_enabled_at IS NOT NULL, outbound_enabled_at IS NOT NULL, issue_sync_enabled_at IS NOT NULL
		 FROM integration_project_binding WHERE project_id=$1 AND connection_id=$2`,
		projectID, uuidToString(conn.ID)).Scan(&inbound, &outbound, &issueSync); err != nil {
		t.Fatalf("load binding: %v", err)
	}
	if !inbound {
		t.Fatalf("execution binding not inbound-enabled")
	}
	if !outbound {
		t.Fatalf("execution binding stuck inbound-only: outbound_enabled_at is NULL")
	}
	if !issueSync {
		t.Fatalf("execution binding issue_sync not enabled")
	}
}
