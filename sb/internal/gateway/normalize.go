package gateway

import (
	"encoding/json"
	"log"
)

// CollapseConsecutiveRoles merges consecutive messages with the same role to
// satisfy Anthropic's strict user → assistant → user alternation requirement.
//
// This is a defensive pass. In Phase 1a BYOBrain only issues single-role turns
// (CRUD operations — no multi-turn tool loops), so this function should rarely
// fire. A warning is logged when it does merge, making the assumption observable
// without blocking the request.
//
// Merging rules:
//   - "tool" role messages are never merged — each is a distinct call result
//     and must be kept separate so the upstream provider can correlate them.
//   - Consecutive same-role text messages have their content concatenated with
//     a double newline separator (preserves paragraph structure).
//   - ToolCalls on consecutive assistant messages are unioned (appended).
func CollapseConsecutiveRoles(msgs []Message) []Message {
	if len(msgs) == 0 {
		return msgs
	}

	out := make([]Message, 0, len(msgs))
	out = append(out, msgs[0])

	for i := 1; i < len(msgs); i++ {
		cur := msgs[i]
		prev := &out[len(out)-1]

		// Tool results are always distinct — never merge them, even if consecutive.
		// Each tool result maps 1:1 to a tool_use block in the preceding assistant message.
		if cur.Role == "tool" || prev.Role == "tool" {
			out = append(out, cur)
			continue
		}

		if cur.Role != prev.Role {
			out = append(out, cur)
			continue
		}

		// Same role detected — merge and emit a warning so we can track
		// when conversation construction assumptions change.
		log.Printf("[gateway/normalize] WARN: consecutive %q messages at index %d; merging into %d — check conversation history construction", cur.Role, i, i-1)

		prevText := prev.ContentString()
		curText := cur.ContentString()

		var combined string
		switch {
		case prevText != "" && curText != "":
			combined = prevText + "\n\n" + curText
		case prevText != "":
			combined = prevText
		default:
			combined = curText
		}

		merged, _ := json.Marshal(combined)
		prev.Content = merged

		// Union tool calls (relevant when two consecutive assistant messages both
		// requested tool calls, which is unusual but must be handled gracefully).
		prev.ToolCalls = append(prev.ToolCalls, cur.ToolCalls...)
	}

	return out
}

// intPtr returns a pointer to the given int value.
// Used when setting ToolCall.Index on streaming deltas.
func intPtr(i int) *int { return &i }
