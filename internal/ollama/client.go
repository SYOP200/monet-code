// Package ollama is a small, dependency-free client for the Ollama HTTP API.
//
// It supports the endpoints Monet Code needs: health/version, model listing,
// show, pull with streaming progress, delete, and streaming chat with native
// tool calling (including "thinking" deltas).
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to an Ollama server.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New creates a client for baseURL (e.g. http://127.0.0.1:11434).
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP: &http.Client{
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConns:        8,
				IdleConnTimeout:     30 * time.Second,
				TLSHandshakeTimeout: 5 * time.Second,
			},
		},
	}
}

// Model is an entry from /api/tags.
type Model struct {
	Name       string    `json:"name"`
	Model      string    `json:"model"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	Details    Details   `json:"details"`
}

// Details describes a model's architecture metadata.
type Details struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// DisplaySize renders a byte count as GB/MB.
func DisplaySize(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1f GB", float64(n)/1e9)
	case n > 0:
		return fmt.Sprintf("%.0f MB", float64(n)/1e6)
	default:
		return "—"
	}
}

// APIError is returned for non-2xx responses.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 300 {
		body = body[:300] + "…"
	}
	if body == "" {
		return fmt.Sprintf("ollama: HTTP %d", e.Status)
	}
	return fmt.Sprintf("ollama: HTTP %d: %s", e.Status, body)
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, friendlyNetErr(c.BaseURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &APIError{Status: resp.StatusCode, Body: string(data)}
	}
	return resp, nil
}

func friendlyNetErr(base string, err error) error {
	if strings.Contains(err.Error(), "connection refused") {
		return fmt.Errorf("cannot reach Ollama at %s — is `ollama serve` running?", base)
	}
	return err
}

// Version returns the Ollama server version.
func (c *Client) Version(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	resp, err := c.do(ctx, http.MethodGet, "/api/version", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var v struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	return v.Version, nil
}

// List returns all locally installed models.
func (c *Client) List(ctx context.Context) ([]Model, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/tags", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Models []Model `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Models, nil
}

// Show returns the raw /api/show payload for a model.
func (c *Client) Show(ctx context.Context, name string) (map[string]any, error) {
	resp, err := c.do(ctx, http.MethodPost, "/api/show", map[string]string{"model": name})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes a model from disk.
func (c *Client) Delete(ctx context.Context, name string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/api/delete", map[string]string{"model": name})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// PullProgress is a status update during a model download.
type PullProgress struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
}

// Pull downloads a model, invoking onProgress for every status line.
func (c *Client) Pull(ctx context.Context, name string, onProgress func(PullProgress)) error {
	resp, err := c.do(ctx, http.MethodPost, "/api/pull", map[string]any{"model": name, "stream": true})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var p PullProgress
		if err := json.Unmarshal(line, &p); err != nil {
			continue
		}
		if onProgress != nil {
			onProgress(p)
		}
	}
	return scanner.Err()
}

// Role constants for chat messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// FunctionCall is the function portion of a tool call.
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	ID       string       `json:"id,omitempty"`
	Function FunctionCall `json:"function"`
}

// Message is one chat message.
type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	Thinking  string     `json:"thinking,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
	Images    []string   `json:"images,omitempty"`
}

// Tool is a tool declaration sent to the model.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable tool.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ChatRequest is a /api/chat payload.
type ChatRequest struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Tools    []Tool         `json:"tools,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
	Format   string         `json:"format,omitempty"`
}

// ChatChunk is one streamed response chunk.
type ChatChunk struct {
	Model   string  `json:"model"`
	Message Message `json:"message"`
	Done    bool    `json:"done"`
	Error   string  `json:"error,omitempty"`
}

// Chat streams a chat completion, calling onChunk for each chunk. Returning an
// error from onChunk aborts the request.
func (c *Client) Chat(ctx context.Context, req ChatRequest, onChunk func(ChatChunk) error) error {
	req.Stream = true
	resp, err := c.do(ctx, http.MethodPost, "/api/chat", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var chunk ChatChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}
		if chunk.Error != "" {
			return fmt.Errorf("%s", chunk.Error)
		}
		if err := onChunk(chunk); err != nil {
			return err
		}
		if chunk.Done {
			return nil
		}
	}
	return scanner.Err()
}
