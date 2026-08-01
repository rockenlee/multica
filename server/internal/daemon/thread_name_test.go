package daemon

import (
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/pkg/agent"
)

func TestDeriveTaskThreadNamePrefersClaimedThreadName(t *testing.T) {
	t.Parallel()

	got := deriveTaskThreadName(Task{
		ThreadName:            "  Fix login redirect  ",
		TriggerCommentContent: "please look at this comment",
		ChatMessage:           "chat fallback",
	})
	if got != "Fix login redirect" {
		t.Fatalf("thread name = %q, want %q", got, "Fix login redirect")
	}
}

func TestDeriveTaskThreadNameFallsBackToTaskContext(t *testing.T) {
	t.Parallel()

	got := deriveTaskThreadName(Task{QuickCreatePrompt: "create issue for billing sync"})
	if got != "create issue for billing sync" {
		t.Fatalf("thread name = %q, want quick-create prompt", got)
	}
}

func TestNormalizeThreadNameCollapsesWhitespaceAndTruncates(t *testing.T) {
	t.Parallel()

	input := "first line\n\t" + strings.Repeat("x", codexThreadNameMaxRunes+20)
	got := normalizeThreadName(input, codexThreadNameMaxRunes)
	if strings.ContainsAny(got, "\n\t") {
		t.Fatalf("thread name still contains raw whitespace: %q", got)
	}
	if len([]rune(got)) != codexThreadNameMaxRunes {
		t.Fatalf("thread name rune length = %d, want %d", len([]rune(got)), codexThreadNameMaxRunes)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("thread name should end with ellipsis marker, got %q", got)
	}
}

func TestDeriveCodexThreadModeVisibleForIssueAndResumableTasks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		task Task
	}{
		{name: "issue", task: Task{IssueID: "issue-1"}},
		{name: "chat", task: Task{ChatSessionID: "chat-1"}},
		{name: "autopilot", task: Task{AutopilotRunID: "run-1"}},
		{name: "prior session", task: Task{PriorSessionID: "thr-prior"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := deriveCodexThreadMode(tc.task); got != agent.CodexThreadModeVisible {
				t.Fatalf("thread mode = %q, want %q", got, agent.CodexThreadModeVisible)
			}
		})
	}
}

func TestDeriveCodexThreadModeDefaultsToEphemeral(t *testing.T) {
	t.Parallel()

	got := deriveCodexThreadMode(Task{QuickCreatePrompt: "create an issue from this note"})
	if got != agent.CodexThreadModeEphemeral {
		t.Fatalf("thread mode = %q, want %q", got, agent.CodexThreadModeEphemeral)
	}
}
