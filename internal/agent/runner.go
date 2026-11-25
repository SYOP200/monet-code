package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/syop200/monet-code/internal/files"
	"github.com/syop200/monet-code/internal/ollama"
)

// EventType enumerates runner events consumed by the TUI.
type EventType string

const (
	EventDelta      EventType = "delta"
	EventThinking   EventType = "thinking"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventApproval   EventType = "approval"
	EventNotice     EventType = "notice"
	EventError      EventType = "error"
	EventDone       EventType = "done"
)

// Event is one step of an agent turn.
type Event struct {
	Type       EventType
	Gen        int
	Text       string // content delta, notice, error message, approval summary
	ToolName   string
	ToolArgs   string
	ToolResult string
}

// Runner drives a single conversation, streaming events and executing tools.
type Runner struct {
	client *ollama.Client
	exec   *Executor
	root   string

	mu       sync.Mutex
	model    string
	system   string
	messages []ollama.Message
	gen      int
	running  bool
	cancel   context.CancelFunc

	events    chan Event
	approvals chan bool

	// MaxSteps caps tool-calling rounds per user turn.
	MaxSteps int
}

// NewRunner creates a runner for root using the given Ollama client.
func NewRunner(client *ollama.Client, root, model string) *Runner {
	return &Runner{
		client:    client,
		exec:      NewExecutor(root),
		root:      root,
		model:     model,
		system:    BuildSystemPrompt(root),
		events:    make(chan Event, 512),
		approvals: make(chan bool, 1),
		MaxSteps:  10,
	}
}

// Events is the stream of turn events.
func (r *Runner) Events() <-chan Event { return r.events }

// SetModel changes the active model for subsequent turns.
func (r *Runner) SetModel(model string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.model = model
}

// Model returns the active model.
func (r *Runner) Model() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.model
}

// SetSystemPrompt replaces the system prompt for subsequent turns.
func (r *Runner) SetSystemPrompt(prompt string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.system = prompt
}

// SystemPrompt describes the workspace to the model.
func BuildSystemPrompt(root string) string {
	summary := files.SummaryTree(root, 40)
	var b strings.Builder
	b.WriteString("You are Monet Code, an AI coding agent living in a terminal TUI.\n")
	fmt.Fprintf(&b, "Workspace root: %s\n", root)
	if summary != "" {
		fmt.Fprintf(&b, "Top-level entries: %s\n", summary)
	}
	b.WriteString(`
Operate as a careful pair programmer:
- Inspect before you edit. Use read_file, list_files and search_files to understand the code.
- Use write_file with the COMPLETE new file content when making changes. Never use placeholders like "// rest unchanged".
- Prefer small, focused changes. Preserve the surrounding style, naming and formatting.
- Paths are relative to the workspace; never try to escape it.
- Tool calls that modify files require the user's approval and may be denied.
- After tool results, explain what changed in one or two short sentences.
- Answer in Markdown: short paragraphs, bullet lists, fenced code blocks with language tags.
- Be direct and concise. If you are unsure, say so and ask one focused question.
`)
	b.WriteString("Current date: " + time.Now().Format("2006-01-02") + "\n")
	return b.String()
}

// Running reports whether a turn is in flight.
func (r *Runner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// Send starts a turn with prompt. It returns the generation id used to tag
// events; callers should ignore events from older generations.
func (r *Runner) Send(prompt string) (int, error) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return 0, fmt.Errorf("a turn is already running")
	}
	if strings.TrimSpace(prompt) == "" {
		r.mu.Unlock()
		return 0, fmt.Errorf("empty prompt")
	}
	r.gen++
	gen := r.gen
	r.running = true
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.messages = append(r.messages, ollama.Message{Role: ollama.RoleUser, Content: prompt})
	r.mu.Unlock()

	go r.loop(ctx, gen)
	return gen, nil
}

// Cancel stops the current turn.
func (r *Runner) Cancel() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Approve answers a pending write-approval request. A stale response with no
// pending request is discarded.
func (r *Runner) Approve(ok bool) {
	select {
	case r.approvals <- ok:
	default:
	}
}

// Clear resets the conversation history.
func (r *Runner) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = nil
}

// History returns a copy of the conversation (without the system prompt).
func (r *Runner) History() []ollama.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ollama.Message, len(r.messages))
	copy(out, r.messages)
	return out
}

func (r *Runner) emit(ctx context.Context, ev Event) bool {
	select {
	case r.events <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *Runner) emitFinal(ev Event) {
	select {
	case r.events <- ev:
	default:
	}
}

func (r *Runner) loop(ctx context.Context, gen int) {
	defer func() {
		r.mu.Lock()
		r.running = false
		r.cancel = nil
		r.mu.Unlock()
		r.emitFinal(Event{Type: EventDone, Gen: gen})
	}()

	for step := 0; step < r.MaxSteps; step++ {
		r.mu.Lock()
		model := r.model
		system := r.system
		history := make([]ollama.Message, len(r.messages))
		copy(history, r.messages)
		r.mu.Unlock()

		msgs := make([]ollama.Message, 0, len(history)+1)
		msgs = append(msgs, ollama.Message{Role: ollama.RoleSystem, Content: system})
		msgs = append(msgs, history...)

		req := ollama.ChatRequest{
			Model:    model,
			Messages: msgs,
			Tools:    r.exec.Definitions(),
			Options:  map[string]any{"temperature": 0.3, "num_ctx": 16384},
		}

		var content, thinking strings.Builder
		var calls []ollama.ToolCall

		err := r.client.Chat(ctx, req, func(chunk ollama.ChatChunk) error {
			if chunk.Message.Thinking != "" {
				thinking.WriteString(chunk.Message.Thinking)
				if !r.emit(ctx, Event{Type: EventThinking, Gen: gen, Text: chunk.Message.Thinking}) {
					return ctx.Err()
				}
			}
			if chunk.Message.Content != "" {
				content.WriteString(chunk.Message.Content)
				if !r.emit(ctx, Event{Type: EventDelta, Gen: gen, Text: chunk.Message.Content}) {
					return ctx.Err()
				}
			}
			if len(chunk.Message.ToolCalls) > 0 {
				calls = append(calls, chunk.Message.ToolCalls...)
			}
			return nil
		})

		cancelled := ctx.Err() != nil
		if content.Len() > 0 || len(calls) > 0 {
			msg := ollama.Message{Role: ollama.RoleAssistant, Content: content.String()}
			if thinking.Len() > 0 {
				msg.Thinking = thinking.String()
			}
			msg.ToolCalls = calls
			r.mu.Lock()
			r.messages = append(r.messages, msg)
			r.mu.Unlock()
		}
		if err != nil {
			if cancelled {
				return
			}
			r.emitFinal(Event{Type: EventError, Gen: gen, Text: err.Error()})
			return
		}
		if len(calls) == 0 {
			return
		}

		for _, call := range calls {
			name := call.Function.Name
			if name == "" {
				continue
			}
			args := string(call.Function.Arguments)
			summary := r.exec.DescribeCall(name, call.Function.Arguments)
			if !r.emit(ctx, Event{Type: EventToolCall, Gen: gen, ToolName: name, ToolArgs: args, Text: summary}) {
				return
			}
			if r.exec.RequiresApproval(name) {
				if !r.emit(ctx, Event{Type: EventApproval, Gen: gen, ToolName: name, ToolArgs: args, Text: summary}) {
					return
				}
				ok := false
				select {
				case ok = <-r.approvals:
				case <-ctx.Done():
					return
				}
				if !ok {
					note := "The user denied this tool call. Do not retry it; acknowledge the denial and ask how to proceed."
					r.mu.Lock()
					r.messages = append(r.messages, ollama.Message{Role: ollama.RoleTool, ToolName: name, Content: note})
					r.mu.Unlock()
					r.emit(ctx, Event{Type: EventToolResult, Gen: gen, ToolName: name, Text: summary, ToolResult: "denied by user"})
					continue
				}
			}
			result, execErr := r.exec.Execute(name, call.Function.Arguments)
			if execErr != nil {
				result = "error: " + execErr.Error()
			}
			r.mu.Lock()
			r.messages = append(r.messages, ollama.Message{Role: ollama.RoleTool, ToolName: name, Content: result})
			r.mu.Unlock()
			preview := result
			if len(preview) > 2000 {
				preview = preview[:2000] + "\n… (truncated)"
			}
			if !r.emit(ctx, Event{Type: EventToolResult, Gen: gen, ToolName: name, Text: summary, ToolResult: preview}) {
				return
			}
		}
	}
	r.emitFinal(Event{Type: EventNotice, Gen: gen, Text: fmt.Sprintf("tool loop stopped after %d rounds", r.MaxSteps)})
}

// PrettyArgs renders tool arguments for display.
func PrettyArgs(raw string) string {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	data, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	s := string(data)
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}
