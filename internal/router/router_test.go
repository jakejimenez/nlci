package router

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jakejimenez/nlci/internal/backend"
)

type stubBackend struct {
	name      string
	responses []string // returned in order; last one is reused if exhausted
	calls     int
	lastReq   backend.Request
}

func (s *stubBackend) Name() string                                  { return s.name }
func (s *stubBackend) Ping(ctx context.Context) error                { return nil }
func (s *stubBackend) Generate(ctx context.Context, r backend.Request) (backend.Response, error) {
	s.lastReq = r
	idx := s.calls
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	s.calls++
	if idx < 0 {
		return backend.Response{}, errors.New("no responses configured")
	}
	return backend.Response{Command: s.responses[idx]}, nil
}

func newRouter(stub *stubBackend) *backend.Router {
	return backend.NewRouterWithBackends([]backend.Backend{stub})
}

func makeTools() []ToolMeta {
	return []ToolMeta{
		{Name: "docker", Description: "Docker container and image management", Synonyms: []string{"container", "image"}},
		{Name: "gh", Description: "GitHub CLI for repos, PRs, issues", Synonyms: []string{"github", "pr"}},
		{Name: "curl", Description: "Transfer data from or to a server", Synonyms: []string{"http", "request"}},
	}
}

func TestRoute_HappyPath(t *testing.T) {
	stub := &stubBackend{name: "stub", responses: []string{"docker"}}
	got, err := Route(context.Background(), "list my containers", makeTools(), newRouter(stub))
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if got != "docker" {
		t.Fatalf("want docker, got %q", got)
	}
	if stub.calls != 1 {
		t.Fatalf("expected 1 backend call, got %d", stub.calls)
	}
}

func TestRoute_NormalizesOutput(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"docker", "docker"},
		{"DOCKER", "docker"},
		{"`docker`", "docker"},
		{"  docker\n", "docker"},
		{"- docker", "docker"},
		{"docker — for containers", "docker"},
		{"docker.", "docker"},
	}
	for _, tc := range cases {
		stub := &stubBackend{name: "stub", responses: []string{tc.raw}}
		got, err := Route(context.Background(), "x", makeTools(), newRouter(stub))
		if err != nil {
			t.Fatalf("Route(%q): %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("Route(%q): want %q, got %q", tc.raw, tc.want, got)
		}
	}
}

func TestRoute_RetryOnInvalid(t *testing.T) {
	stub := &stubBackend{name: "stub", responses: []string{"unknown-tool", "gh"}}
	got, err := Route(context.Background(), "list my prs", makeTools(), newRouter(stub))
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if got != "gh" {
		t.Fatalf("want gh, got %q", got)
	}
	if stub.calls != 2 {
		t.Fatalf("expected 2 calls (retry), got %d", stub.calls)
	}
	// Retry hint must appear in the second user prompt.
	if !strings.Contains(stub.lastReq.Intent, "previous answer") {
		t.Fatalf("retry user prompt missing hint: %q", stub.lastReq.Intent)
	}
}

func TestRoute_InvalidAfterRetry(t *testing.T) {
	stub := &stubBackend{name: "stub", responses: []string{"unknown1", "unknown2"}}
	_, err := Route(context.Background(), "x", makeTools(), newRouter(stub))
	if !errors.Is(err, ErrInvalidChoice) {
		t.Fatalf("want ErrInvalidChoice, got %v", err)
	}
}

func TestRoute_NoTools(t *testing.T) {
	stub := &stubBackend{name: "stub", responses: []string{"docker"}}
	_, err := Route(context.Background(), "x", nil, newRouter(stub))
	if !errors.Is(err, ErrNoTools) {
		t.Fatalf("want ErrNoTools, got %v", err)
	}
}

func TestRoute_PromptIncludesAllToolNames(t *testing.T) {
	stub := &stubBackend{name: "stub", responses: []string{"docker"}}
	_, _ = Route(context.Background(), "list containers", makeTools(), newRouter(stub))
	for _, name := range []string{"docker", "gh", "curl"} {
		if !strings.Contains(stub.lastReq.System, name) {
			t.Fatalf("expected system prompt to mention %q: %s", name, stub.lastReq.System)
		}
	}
}
