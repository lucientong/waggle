package guardrail

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lucientong/waggle/pkg/agent"
)

func TestMaxLength(t *testing.T) {
	v := MaxLength(5)
	if err := v.Validate("hello"); err != nil {
		t.Errorf("should pass: %v", err)
	}
	if err := v.Validate("toolong"); err == nil {
		t.Error("should fail for 7 chars > 5")
	}
}

func TestMinLength(t *testing.T) {
	v := MinLength(3)
	if err := v.Validate("abc"); err != nil {
		t.Errorf("should pass: %v", err)
	}
	if err := v.Validate("ab"); err == nil {
		t.Error("should fail for 2 chars < 3")
	}
}

func TestMaxLength_Unicode(t *testing.T) {
	v := MaxLength(3)
	if err := v.Validate("你好吗"); err != nil {
		t.Errorf("3 runes should pass: %v", err)
	}
	if err := v.Validate("你好吗啊"); err == nil {
		t.Error("4 runes should fail")
	}
}

func TestRegexMatch(t *testing.T) {
	v := RegexMatch(`^\d+$`)
	if err := v.Validate("123"); err != nil {
		t.Errorf("should match: %v", err)
	}
	if err := v.Validate("abc"); err == nil {
		t.Error("should not match")
	}
}

func TestRegexReject(t *testing.T) {
	v := RegexReject(`password`, "password leak")
	if err := v.Validate("hello world"); err != nil {
		t.Errorf("should pass: %v", err)
	}
	if err := v.Validate("my password is 123"); err == nil {
		t.Error("should reject")
	}
}

func TestJSONValid(t *testing.T) {
	v := JSONValid()

	valid := []string{
		`{"key": "value"}`,
		`[1, 2, 3]`,
		`{"nested": {"a": [1]}}`,
	}
	for _, s := range valid {
		if err := v.Validate(s); err != nil {
			t.Errorf("should be valid JSON: %q → %v", s, err)
		}
	}

	invalid := []string{
		`not json`,
		``,
		`{"unclosed": true`,
	}
	for _, s := range invalid {
		if err := v.Validate(s); err == nil {
			t.Errorf("should be invalid JSON: %q", s)
		}
	}
}

func TestContentFilter(t *testing.T) {
	v := ContentFilter([]string{"password", "secret"})
	if err := v.Validate("hello world"); err != nil {
		t.Errorf("should pass: %v", err)
	}
	if err := v.Validate("my PASSWORD is here"); err == nil {
		t.Error("should reject case-insensitive match")
	}
	if err := v.Validate("this is a secret"); err == nil {
		t.Error("should reject")
	}
}

func TestPIIEmail(t *testing.T) {
	if err := PIIEmail.Validate("contact me at user@example.com"); err == nil {
		t.Error("should detect email")
	}
	if err := PIIEmail.Validate("no email here"); err != nil {
		t.Errorf("should pass: %v", err)
	}
}

func TestPIIPhone(t *testing.T) {
	if err := PIIPhone.Validate("call me at 555-123-4567"); err == nil {
		t.Error("should detect phone")
	}
	if err := PIIPhone.Validate("no phone here"); err != nil {
		t.Errorf("should pass: %v", err)
	}
}

func TestPIISSN(t *testing.T) {
	if err := PIISSNLike.Validate("SSN: 123-45-6789"); err == nil {
		t.Error("should detect SSN")
	}
	if err := PIISSNLike.Validate("no ssn"); err != nil {
		t.Errorf("should pass: %v", err)
	}
}

func TestWithInputGuard(t *testing.T) {
	inner := agent.Func[string, string]("echo", func(_ context.Context, s string) (string, error) {
		return s, nil
	})

	guarded := WithInputGuard(inner, MaxLength(5))

	// Valid input.
	result, err := guarded.Run(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello" {
		t.Errorf("unexpected: %q", result)
	}

	// Invalid input.
	_, err = guarded.Run(context.Background(), "toolong")
	if err == nil {
		t.Fatal("expected guard violation")
	}
	var gve *GuardViolationError
	if !errors.As(err, &gve) {
		t.Fatalf("expected GuardViolationError, got %T", err)
	}
	if gve.Phase != "input" {
		t.Errorf("expected input phase, got %s", gve.Phase)
	}
	if gve.AgentName != "echo" {
		t.Errorf("expected agent name echo, got %s", gve.AgentName)
	}
}

func TestWithOutputGuard(t *testing.T) {
	inner := agent.Func[string, string]("upper", func(_ context.Context, s string) (string, error) {
		return strings.ToUpper(s), nil
	})

	guarded := WithOutputGuard(inner, MaxLength(5))

	// Valid output.
	result, err := guarded.Run(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if result != "HI" {
		t.Errorf("unexpected: %q", result)
	}

	// Invalid output.
	_, err = guarded.Run(context.Background(), "toolong")
	if err == nil {
		t.Fatal("expected guard violation")
	}
	var gve *GuardViolationError
	if !errors.As(err, &gve) {
		t.Fatalf("expected GuardViolationError, got %T", err)
	}
	if gve.Phase != "output" {
		t.Errorf("expected output phase, got %s", gve.Phase)
	}
}

func TestWithInputGuard_MultipleValidators(t *testing.T) {
	inner := agent.Func[string, string]("echo", func(_ context.Context, s string) (string, error) {
		return s, nil
	})

	guarded := WithInputGuard(inner, MinLength(2), MaxLength(10))

	if _, err := guarded.Run(context.Background(), "a"); err == nil {
		t.Error("should fail min length")
	}
	if _, err := guarded.Run(context.Background(), "this is way too long"); err == nil {
		t.Error("should fail max length")
	}
	if _, err := guarded.Run(context.Background(), "good"); err != nil {
		t.Errorf("should pass: %v", err)
	}
}

func TestWithOutputGuard_InnerError(t *testing.T) {
	inner := agent.Func[string, string]("fail", func(_ context.Context, _ string) (string, error) {
		return "", errors.New("inner error")
	})

	guarded := WithOutputGuard(inner, MaxLength(100))

	_, err := guarded.Run(context.Background(), "test")
	if err == nil || err.Error() != "inner error" {
		t.Errorf("expected inner error, got: %v", err)
	}
}

func TestGuardViolationError_Unwrap(t *testing.T) {
	inner := errors.New("too long")
	gve := &GuardViolationError{
		AgentName:     "test",
		ValidatorName: "max_length",
		Phase:         "input",
		Err:           inner,
	}
	if !errors.Is(gve, inner) {
		t.Error("Unwrap should work")
	}
}

func TestNewValidator(t *testing.T) {
	v := NewValidator("custom", func(s string) error {
		if s == "bad" {
			return errors.New("bad value")
		}
		return nil
	})
	if v.Name() != "custom" {
		t.Errorf("expected name custom, got %s", v.Name())
	}
	if err := v.Validate("good"); err != nil {
		t.Errorf("should pass: %v", err)
	}
	if err := v.Validate("bad"); err == nil {
		t.Error("should fail")
	}
}

func TestGuardedAgent_Name(t *testing.T) {
	inner := agent.Func[string, string]("myagent", func(_ context.Context, s string) (string, error) {
		return s, nil
	})
	ig := WithInputGuard(inner, MaxLength(10))
	og := WithOutputGuard(inner, MaxLength(10))
	if ig.Name() != "myagent" {
		t.Errorf("expected myagent, got %s", ig.Name())
	}
	if og.Name() != "myagent" {
		t.Errorf("expected myagent, got %s", og.Name())
	}
}
