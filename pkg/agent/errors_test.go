package agent_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/lucientong/waggle/pkg/agent"
)

func TestErrTypeMismatch_Error(t *testing.T) {
	err := &agent.ErrTypeMismatch{
		AgentName: "my-agent",
		Got:       42,
	}
	msg := err.Error()
	if !strings.Contains(msg, "my-agent") {
		t.Errorf("Error() = %q, want to contain agent name", msg)
	}
	if !strings.Contains(msg, "type mismatch") {
		t.Errorf("Error() = %q, want to contain 'type mismatch'", msg)
	}
	if !strings.Contains(msg, "int") {
		t.Errorf("Error() = %q, want to contain type 'int'", msg)
	}
}

func TestRetryExhaustedError_Error(t *testing.T) {
	inner := errors.New("connection refused")
	err := &agent.RetryExhaustedError{
		AgentName: "flaky-agent",
		Attempts:  5,
		LastErr:   inner,
	}
	msg := err.Error()
	if !strings.Contains(msg, "flaky-agent") {
		t.Errorf("Error() = %q, want to contain agent name", msg)
	}
	if !strings.Contains(msg, "5") {
		t.Errorf("Error() = %q, want to contain attempt count", msg)
	}
	if !strings.Contains(msg, "connection refused") {
		t.Errorf("Error() = %q, want to contain last error", msg)
	}

	// Test Unwrap.
	if !errors.Is(err, inner) {
		t.Error("errors.Is(err, inner) should be true")
	}
}
