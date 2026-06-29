package zentao

import "testing"

// TestZentaoActionForStatus verifies the target-status → real-action mapping and
// the per-action effort fields that ZenTao requires (left>0 for start,
// consumed>0 + left=0 for finish). This is the fix for outbound status sync
// 400-ing / silently no-op-ing via the generic task PUT. Pure function — no DB.
const testToday = "2026-06-29"

func TestZentaoActionForStatus(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		task       map[string]any
		wantAction string
		check      func(t *testing.T, p map[string]any)
	}{
		{
			name:       "doing -> start, left from current left + realStarted defaults to today",
			target:     "doing",
			task:       map[string]any{"left": "3", "estimate": "8"},
			wantAction: "start",
			check: func(t *testing.T, p map[string]any) {
				if p["left"].(float64) != 3 {
					t.Fatalf("start left = %v, want 3", p["left"])
				}
				if p["realStarted"] != testToday {
					t.Fatalf("start realStarted = %v, want %s (today fallback)", p["realStarted"], testToday)
				}
			},
		},
		{
			name:       "doing -> start, left falls back to estimate when left is 0",
			target:     "doing",
			task:       map[string]any{"left": "0", "estimate": "8"},
			wantAction: "start",
			check: func(t *testing.T, p map[string]any) {
				if p["left"].(float64) != 8 {
					t.Fatalf("start left = %v, want 8 (estimate)", p["left"])
				}
				if p["realStarted"] != testToday {
					t.Fatalf("start realStarted = %v, want %s", p["realStarted"], testToday)
				}
			},
		},
		{
			name:       "doing -> start, left defaults to 1; realStarted uses task value (date part)",
			target:     "doing",
			task:       map[string]any{"realStarted": "2026-06-01 10:30:00"},
			wantAction: "start",
			check: func(t *testing.T, p map[string]any) {
				if p["left"].(float64) != 1 {
					t.Fatalf("start left = %v, want 1", p["left"])
				}
				if p["realStarted"] != "2026-06-01" {
					t.Fatalf("start realStarted = %v, want 2026-06-01 (task value, date part)", p["realStarted"])
				}
			},
		},
		{
			name:       "done -> finish, consumed/currentConsumed>0, left 0, realStarted+finishedDate present",
			target:     "done",
			task:       map[string]any{"consumed": "0.00", "estimate": "5", "realStarted": "2026-06-01", "finishedDate": "0000-00-00 00:00:00"},
			wantAction: "finish",
			check: func(t *testing.T, p map[string]any) {
				if p["consumed"].(float64) != 5 {
					t.Fatalf("finish consumed = %v, want 5 (estimate)", p["consumed"])
				}
				if p["currentConsumed"].(float64) != 5 {
					t.Fatalf("finish currentConsumed = %v, want 5", p["currentConsumed"])
				}
				if p["left"] != 0 {
					t.Fatalf("finish left = %v, want 0", p["left"])
				}
				if p["realStarted"] != "2026-06-01" {
					t.Fatalf("finish realStarted = %v, want 2026-06-01 (task value)", p["realStarted"])
				}
				if p["finishedDate"] != testToday {
					t.Fatalf("finish finishedDate = %v, want %s (today, task value was 0000-00-00)", p["finishedDate"], testToday)
				}
			},
		},
		{
			name:       "done -> finish, all dates default to today when task has none",
			target:     "done",
			task:       map[string]any{},
			wantAction: "finish",
			check: func(t *testing.T, p map[string]any) {
				if p["currentConsumed"].(float64) != 1 {
					t.Fatalf("finish currentConsumed = %v, want 1 (default)", p["currentConsumed"])
				}
				if p["realStarted"] != testToday || p["finishedDate"] != testToday {
					t.Fatalf("finish dates = realStarted %v / finishedDate %v, want both %s", p["realStarted"], p["finishedDate"], testToday)
				}
			},
		},
		{
			// cancel (delete path) must NOT use the unsupported /cancel action
			// endpoint; it falls back to the generic full-form PUT with status=cancel.
			name:       "cancelled -> generic edit with status=cancel (no /cancel endpoint)",
			target:     "cancel",
			task:       map[string]any{"name": "t", "execution": "447", "type": "devel", "consumed": "2"},
			wantAction: zentaoEditStatus,
			check: func(t *testing.T, p map[string]any) {
				if p["status"] != "cancel" {
					t.Fatalf("cancel payload status = %v, want \"cancel\"", p["status"])
				}
				if p["name"] != "t" || p["execution"] != "447" || p["type"] != "devel" {
					t.Fatalf("cancel payload missing revalidated form fields: %#v", p)
				}
			},
		},
		{
			// blocked -> pause has no /pause endpoint in ZenTao 21.7.8; skip explicitly
			// (empty action) rather than calling a missing route.
			name:       "blocked -> pause is unsupported, skip (no /pause endpoint)",
			target:     "pause",
			task:       map[string]any{},
			wantAction: "",
		},
		{name: "closed -> close", target: "closed", task: map[string]any{}, wantAction: "close"},
		{name: "wait -> no direct action", target: "wait", task: map[string]any{}, wantAction: ""},
		{name: "unknown -> no direct action", target: "weird", task: map[string]any{}, wantAction: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			action, payload := zentaoActionForStatus(c.target, c.task, testToday)
			if action != c.wantAction {
				t.Fatalf("action = %q, want %q", action, c.wantAction)
			}
			if c.check != nil {
				c.check(t, payload)
			}
		})
	}
}

func TestTaskNumAndFirstPositive(t *testing.T) {
	if taskNum("8") != 8 || taskNum(float64(3)) != 3 || taskNum("0.00") != 0 || taskNum(nil) != 0 {
		t.Fatalf("taskNum parsing wrong")
	}
	if firstPositive(0, 0, 5) != 5 {
		t.Fatalf("firstPositive should pick first >0")
	}
	if firstPositive(0, 0, 0) != 0 {
		t.Fatalf("firstPositive all-zero should be 0")
	}
}
