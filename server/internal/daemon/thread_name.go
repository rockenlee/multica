package daemon

import (
	"strings"

	"github.com/multica-ai/multica/server/pkg/agent"
)

const codexThreadNameMaxRunes = 120

func deriveTaskThreadName(task Task) string {
	candidates := []string{
		task.ThreadName,
		task.AutopilotTitle,
		task.QuickCreatePrompt,
		task.ChatMessage,
		task.TriggerCommentContent,
	}
	for _, candidate := range candidates {
		if name := normalizeThreadName(candidate, codexThreadNameMaxRunes); name != "" {
			return name
		}
	}
	return ""
}

func deriveCodexThreadMode(task Task) agent.CodexThreadMode {
	if task.IssueID != "" || task.ChatSessionID != "" || task.AutopilotRunID != "" || task.PriorSessionID != "" {
		return agent.CodexThreadModeVisible
	}
	return agent.CodexThreadModeEphemeral
}

func normalizeThreadName(s string, maxRunes int) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	normalized := strings.Join(fields, " ")
	if maxRunes <= 0 {
		return normalized
	}
	rs := []rune(normalized)
	if len(rs) <= maxRunes {
		return normalized
	}
	if maxRunes <= 3 {
		return string(rs[:maxRunes])
	}
	return string(rs[:maxRunes-3]) + "..."
}
