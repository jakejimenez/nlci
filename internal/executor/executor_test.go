package executor

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

// withCapturedStdout swaps os.Stdout for a pipe, runs fn, and returns
// everything fn wrote.
func withCapturedStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func TestExecute_DryRun_ShortCircuits(t *testing.T) {
	out := withCapturedStdout(t, func() {
		err := Execute(context.Background(), "echo would-not-run", "", Options{DryRun: true})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "[dry-run: command not executed]") {
		t.Fatalf("expected dry-run marker in output: %q", out)
	}
	if strings.Contains(out, "would-not-run\n") && !strings.Contains(out, "> echo would-not-run") {
		t.Fatalf("dry-run leaked actual command output: %q", out)
	}
}

func TestExecute_AutoConfirm_RunsWithoutPrompt(t *testing.T) {
	out := withCapturedStdout(t, func() {
		err := Execute(context.Background(), "echo ran-it", "", Options{AutoConfirm: true})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})
	if strings.Contains(out, "Run this command?") {
		t.Fatalf("AutoConfirm should suppress prompt, got: %q", out)
	}
	if !strings.Contains(out, "ran-it") {
		t.Fatalf("expected subprocess output, got: %q", out)
	}
}

func TestExecute_DefaultPrompts_DeclineCancels(t *testing.T) {
	// Pipe an empty stdin so Confirm reads EOF and declines.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = w.Close()
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	out := withCapturedStdout(t, func() {
		err := Execute(context.Background(), "echo would-cancel", "", Options{})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "Run this command?") {
		t.Fatalf("default mode should prompt, got: %q", out)
	}
	if !strings.Contains(out, "Cancelled") {
		t.Fatalf("expected cancel message, got: %q", out)
	}
	if strings.Contains(out, "would-cancel\n") && !strings.Contains(out, "> echo would-cancel") {
		t.Fatalf("declined command should not have produced subprocess output: %q", out)
	}
}

func TestExecute_DefaultPrompts_AcceptRuns(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_, _ = w.WriteString("y\n")
	_ = w.Close()
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	out := withCapturedStdout(t, func() {
		err := Execute(context.Background(), "echo accepted", "", Options{})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, "Run this command?") {
		t.Fatalf("default mode should prompt, got: %q", out)
	}
	if !strings.Contains(out, "accepted") {
		t.Fatalf("expected subprocess output after y, got: %q", out)
	}
}
