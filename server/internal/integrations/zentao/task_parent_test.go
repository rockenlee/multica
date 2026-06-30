package zentao

import (
	"encoding/json"
	"testing"
)

func TestTaskUnmarshalParentIDVariants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "parent string", raw: `{"id":"10","name":"child","parent":"9"}`, want: "9"},
		{name: "parentID number", raw: `{"id":10,"name":"child","parentID":9}`, want: "9"},
		{name: "parent_id string", raw: `{"id":"10","name":"child","parent_id":"9"}`, want: "9"},
		{name: "zero ignored", raw: `{"id":"10","name":"root","parent":"0"}`, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var task Task
			if err := json.Unmarshal([]byte(tc.raw), &task); err != nil {
				t.Fatalf("unmarshal task: %v", err)
			}
			if task.ParentID != tc.want {
				t.Fatalf("ParentID = %q, want %q", task.ParentID, tc.want)
			}
		})
	}
}

func TestTaskUnmarshalUpdatedAtVariants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "done prefers finishedDate",
			raw:  `{"id":"10","name":"task","status":"done","lastEditedDate":"2026-06-29 08:00:00","finishedDate":"2026-06-29 09:30:00"}`,
			want: "2026-06-29 09:30:00",
		},
		{
			name: "closed prefers closedDate",
			raw:  `{"id":"10","name":"task","status":"closed","updatedAt":"2026-06-29 08:00:00","closedDate":"2026-06-29 10:00:00"}`,
			want: "2026-06-29 10:00:00",
		},
		{
			name: "falls back to last edited",
			raw:  `{"id":"10","name":"task","status":"doing","lastEditedDate":"2026-06-29 08:00:00"}`,
			want: "2026-06-29 08:00:00",
		},
		{
			name: "ignores zero dates",
			raw:  `{"id":"10","name":"task","status":"doing","lastEditedDate":"0000-00-00 00:00:00","openedDate":"2026-06-29 07:00:00"}`,
			want: "2026-06-29 07:00:00",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var task Task
			if err := json.Unmarshal([]byte(tc.raw), &task); err != nil {
				t.Fatalf("unmarshal task: %v", err)
			}
			if task.UpdatedAt != tc.want {
				t.Fatalf("UpdatedAt = %q, want %q", task.UpdatedAt, tc.want)
			}
		})
	}
}
