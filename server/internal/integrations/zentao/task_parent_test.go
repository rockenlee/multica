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
