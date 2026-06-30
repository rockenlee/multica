package feishu

import (
	"encoding/json"
	"testing"
)

func TestTaskExternalStatusFromCompletedAt(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantStatus string
		wantAt     string
	}{
		{
			name:       "completed task",
			raw:        `{"guid":"g1","summary":"done","completed_at":"1782700800000","updated_at":"1782690000000"}`,
			wantStatus: "done",
			wantAt:     "1782700800000",
		},
		{
			name:       "incomplete task",
			raw:        `{"guid":"g2","summary":"todo","completed_at":"0","update_time":1782690000000}`,
			wantStatus: "todo",
			wantAt:     "1782690000000",
		},
		{
			name:       "completed alternate field",
			raw:        `{"guid":"g3","summary":"done","completed_time":1782700800000}`,
			wantStatus: "done",
			wantAt:     "1782700800000",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var task Task
			if err := json.Unmarshal([]byte(tc.raw), &task); err != nil {
				t.Fatalf("unmarshal task: %v", err)
			}
			if got := task.ExternalStatus(); got != tc.wantStatus {
				t.Fatalf("ExternalStatus() = %q, want %q", got, tc.wantStatus)
			}
			if got := task.StatusUpdatedAt(); got != tc.wantAt {
				t.Fatalf("StatusUpdatedAt() = %q, want %q", got, tc.wantAt)
			}
		})
	}
}
