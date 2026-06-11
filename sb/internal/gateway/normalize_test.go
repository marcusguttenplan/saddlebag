package gateway

import (
	"encoding/json"
	"testing"
)

// helpers

func strMsg(role, content string) Message {
	raw, _ := json.Marshal(content)
	return Message{Role: role, Content: raw}
}

func toolResultMsg(toolCallID, content string) Message {
	raw, _ := json.Marshal(content)
	return Message{Role: "tool", ToolCallID: toolCallID, Content: raw}
}

func assistantWithTools(content string, calls ...ToolCall) Message {
	raw, _ := json.Marshal(content)
	return Message{Role: "assistant", Content: raw, ToolCalls: calls}
}

// --- CollapseConsecutiveRoles ---

func TestCollapseConsecutiveRoles_Empty(t *testing.T) {
	out := CollapseConsecutiveRoles(nil)
	if len(out) != 0 {
		t.Fatalf("expected empty, got %d messages", len(out))
	}
}

func TestCollapseConsecutiveRoles_AlreadyAlternating(t *testing.T) {
	msgs := []Message{
		strMsg("user", "hi"),
		strMsg("assistant", "hello"),
		strMsg("user", "how are you"),
	}
	out := CollapseConsecutiveRoles(msgs)
	if len(out) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(out))
	}
	for i, m := range out {
		if m.Role != msgs[i].Role {
			t.Errorf("msg[%d]: expected role %q, got %q", i, msgs[i].Role, m.Role)
		}
	}
}

func TestCollapseConsecutiveRoles_TwoConsecutiveUser(t *testing.T) {
	msgs := []Message{
		strMsg("user", "first"),
		strMsg("user", "second"),
		strMsg("assistant", "ok"),
	}
	out := CollapseConsecutiveRoles(msgs)
	if len(out) != 2 {
		t.Fatalf("expected 2 messages after collapse, got %d", len(out))
	}
	if out[0].Role != "user" {
		t.Errorf("out[0] role: want user, got %s", out[0].Role)
	}
	content := out[0].ContentString()
	if content != "first\n\nsecond" {
		t.Errorf("merged content: want %q, got %q", "first\n\nsecond", content)
	}
	if out[1].Role != "assistant" {
		t.Errorf("out[1] role: want assistant, got %s", out[1].Role)
	}
}

func TestCollapseConsecutiveRoles_TwoConsecutiveAssistant(t *testing.T) {
	tc1 := ToolCall{ID: "tc1", Type: "function", Function: ToolCallFunction{Name: "fn1"}}
	tc2 := ToolCall{ID: "tc2", Type: "function", Function: ToolCallFunction{Name: "fn2"}}

	msgs := []Message{
		strMsg("user", "go"),
		assistantWithTools("thinking...", tc1),
		assistantWithTools("", tc2),
	}
	out := CollapseConsecutiveRoles(msgs)
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
	if len(out[1].ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls after merge, got %d", len(out[1].ToolCalls))
	}
}

func TestCollapseConsecutiveRoles_ToolResultsNeverMerged(t *testing.T) {
	// Consecutive tool results must remain separate — each maps to a distinct tool_use block.
	msgs := []Message{
		strMsg("user", "call two tools"),
		strMsg("assistant", ""),
		toolResultMsg("tc1", "result1"),
		toolResultMsg("tc2", "result2"),
		strMsg("user", "continue"),
	}
	out := CollapseConsecutiveRoles(msgs)
	// Tool results should NOT be merged.
	if len(out) != 5 {
		t.Fatalf("expected 5 messages (tool results kept separate), got %d", len(out))
	}
	if out[2].Role != "tool" || out[3].Role != "tool" {
		t.Error("tool result messages were unexpectedly merged or reordered")
	}
}

func TestCollapseConsecutiveRoles_UserUserAssistant(t *testing.T) {
	msgs := []Message{
		strMsg("user", "A"),
		strMsg("user", "B"),
		strMsg("assistant", "C"),
	}
	out := CollapseConsecutiveRoles(msgs)
	if len(out) != 2 {
		t.Fatalf("want 2, got %d", len(out))
	}
	if out[0].ContentString() != "A\n\nB" {
		t.Errorf("want %q, got %q", "A\n\nB", out[0].ContentString())
	}
}

func TestCollapseConsecutiveRoles_SystemNotMergedWithUser(t *testing.T) {
	// system and user are different roles — should not merge.
	msgs := []Message{
		strMsg("system", "be helpful"),
		strMsg("user", "hello"),
	}
	out := CollapseConsecutiveRoles(msgs)
	if len(out) != 2 {
		t.Fatalf("want 2, got %d", len(out))
	}
}
