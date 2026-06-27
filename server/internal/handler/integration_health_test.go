package handler

import (
	"context"
	"errors"
	"testing"
)

// TestIsIntegrationAuthError covers the classifier that decides whether a sync
// failure is a credential problem the user must fix (→ "action needed") versus
// a transient error that should not nag.
func TestIsIntegrationAuthError(t *testing.T) {
	authErrs := []string{
		`zentao: list tasks HTTP 401: {"error":"Unauthorized"}`,
		`E1004 token has expired`,
		`feishu: invalid_grant: refresh token revoked`,
		`feishu inbound disabled — connection is missing App ID/Secret: no app_secret configured for this connection`,
		`feishu inbound disabled — connection is missing App ID (set it in Integrations)`,
	}
	for _, m := range authErrs {
		if !isIntegrationAuthError(errors.New(m)) {
			t.Errorf("expected auth error for %q", m)
		}
	}
	transient := []string{
		`dial tcp 10.0.0.1:443: connect: connection refused`,
		`context deadline exceeded`,
		`zentao: get task HTTP 500: internal error`,
	}
	for _, m := range transient {
		if isIntegrationAuthError(errors.New(m)) {
			t.Errorf("did not expect auth error for %q", m)
		}
	}
	if isIntegrationAuthError(nil) {
		t.Error("nil error must not be an auth error")
	}
}

// TestRecordInboundAccountHealth proves the health write-back: an auth failure
// flips the account to status='error' with last_error (the signal the Web
// Integrations card + Access Tokens render as "action needed"), a later success
// clears it, and a transient error leaves it untouched.
func TestRecordInboundAccountHealth(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	var connID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO integration_connection (workspace_id, provider, name, status, sync_enabled_at)
		 VALUES ($1, 'zentao', 'health-test-conn', 'active', now()) RETURNING id`,
		testWorkspaceID).Scan(&connID); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM integration_connection WHERE id = $1`, connID) })

	var acctID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO integration_user_account (workspace_id, connection_id, user_id, account_key, account_name, status)
		 VALUES ($1, $2, $3, 'health-test-acct', 'health acct', 'active') RETURNING id`,
		testWorkspaceID, connID, testUserID).Scan(&acctID); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	target := InboundSyncTarget{
		ConnectionID: parseUUID(connID),
		WorkspaceID:  parseUUID(testWorkspaceID),
		UserID:       parseUUID(testUserID),
		Provider:     "zentao",
		AccountKey:   "health-test-acct",
	}
	read := func() (string, *string) {
		var st string
		var le *string
		if err := testPool.QueryRow(ctx, `SELECT status, last_error FROM integration_user_account WHERE id = $1`, acctID).Scan(&st, &le); err != nil {
			t.Fatalf("read account: %v", err)
		}
		return st, le
	}

	// 1) Auth failure → action needed.
	testHandler.recordInboundAccountHealth(ctx, target, errors.New(`zentao: list tasks HTTP 401: {"error":"Unauthorized"}`))
	if st, le := read(); st != "error" || le == nil || *le == "" {
		t.Fatalf("after 401: status=%q last_error=%v, want error + message", st, le)
	}
	t.Log("after 401: status=error, last_error set (UI shows 'action needed')")

	// 2) Recovery → back to active, error cleared.
	testHandler.recordInboundAccountHealth(ctx, target, nil)
	if st, le := read(); st != "active" || le != nil {
		t.Fatalf("after success: status=%q last_error=%v, want active + nil", st, le)
	}
	t.Log("after success: status=active, last_error cleared")

	// 3) Transient error → unchanged (no nag).
	testHandler.recordInboundAccountHealth(ctx, target, errors.New("dial tcp: connection refused"))
	if st, _ := read(); st != "active" {
		t.Fatalf("after transient error: status=%q, want unchanged active", st)
	}
	t.Log("after transient error: status=active (unchanged)")
}
