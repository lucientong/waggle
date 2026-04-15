package conv

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestChannel_SendAndReceive(t *testing.T) {
	ch := NewChannel()
	ch.Send(Envelope{From: "alice", To: "bob", Content: "hello"})
	ch.Send(Envelope{From: "alice", To: "", Content: "broadcast"})

	// Bob should see both (addressed + broadcast).
	msgs := ch.Receive("bob")
	if len(msgs) != 2 {
		t.Fatalf("bob expected 2, got %d", len(msgs))
	}

	// Charlie should see only broadcast.
	msgs = ch.Receive("charlie")
	if len(msgs) != 1 {
		t.Fatalf("charlie expected 1, got %d", len(msgs))
	}
	if msgs[0].Content != "broadcast" {
		t.Errorf("unexpected: %+v", msgs[0])
	}
}

func TestChannel_History(t *testing.T) {
	ch := NewChannel()
	ch.Send(Envelope{From: "a", Content: "1"})
	ch.Send(Envelope{From: "b", Content: "2"})

	history := ch.History()
	if len(history) != 2 {
		t.Fatalf("expected 2, got %d", len(history))
	}

	// Verify it's a copy.
	history[0].Content = "mutated"
	original := ch.History()
	if original[0].Content != "1" {
		t.Error("History should return a copy")
	}
}

func TestChannel_Clear(t *testing.T) {
	ch := NewChannel()
	ch.Send(Envelope{From: "a", Content: "1"})
	ch.Clear()

	if len(ch.History()) != 0 {
		t.Error("expected empty after clear")
	}
}

func TestFuncParticipant(t *testing.T) {
	p := FuncParticipant("echo", func(_ context.Context, incoming []Envelope) (string, error) {
		if len(incoming) == 0 {
			return "no input", nil
		}
		return "echo: " + incoming[len(incoming)-1].Content, nil
	})

	if p.Name() != "echo" {
		t.Errorf("expected name echo, got %s", p.Name())
	}

	env, err := p.Respond(context.Background(), []Envelope{{Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if env.Content != "echo: hello" {
		t.Errorf("unexpected: %+v", env)
	}
}

func TestModerator_BasicConversation(t *testing.T) {
	p1 := FuncParticipant("analyst", func(_ context.Context, incoming []Envelope) (string, error) {
		return fmt.Sprintf("Analysis of %d messages", len(incoming)), nil
	})
	p2 := FuncParticipant("reviewer", func(_ context.Context, incoming []Envelope) (string, error) {
		return "Reviewed", nil
	})

	mod := NewModerator("test", WithMaxRounds(2))
	mod.AddParticipant(p1)
	mod.AddParticipant(p2)

	history, err := mod.Run(context.Background(), "Discuss this topic")
	if err != nil {
		t.Fatal(err)
	}

	// Should have: 1 seed + 2 rounds × 2 participants = 5 messages.
	if len(history) != 5 {
		t.Fatalf("expected 5 messages, got %d: %+v", len(history), history)
	}
	if history[0].From != "moderator" {
		t.Errorf("first message should be from moderator: %+v", history[0])
	}
}

func TestModerator_Termination(t *testing.T) {
	p := FuncParticipant("agent", func(_ context.Context, incoming []Envelope) (string, error) {
		if len(incoming) >= 3 {
			return "DONE", nil
		}
		return "working", nil
	})

	mod := NewModerator("test",
		WithMaxRounds(10),
		WithTermination(func(history []Envelope) bool {
			for _, e := range history {
				if strings.Contains(e.Content, "DONE") {
					return true
				}
			}
			return false
		}),
	)
	mod.AddParticipant(p)

	history, err := mod.Run(context.Background(), "Start")
	if err != nil {
		t.Fatal(err)
	}

	// Should terminate before max rounds.
	lastMsg := history[len(history)-1]
	if !strings.Contains(lastMsg.Content, "DONE") {
		t.Errorf("expected DONE in last message, got %+v", lastMsg)
	}
}

func TestModerator_TurnOrder(t *testing.T) {
	var order []string
	makeP := func(name string) Participant {
		return FuncParticipant(name, func(_ context.Context, _ []Envelope) (string, error) {
			order = append(order, name)
			return name + " says hi", nil
		})
	}

	mod := NewModerator("test",
		WithMaxRounds(1),
		WithTurnOrder([]string{"second", "first"}),
	)
	mod.AddParticipant(makeP("first"))
	mod.AddParticipant(makeP("second"))

	_, err := mod.Run(context.Background(), "topic")
	if err != nil {
		t.Fatal(err)
	}

	if len(order) != 2 || order[0] != "second" || order[1] != "first" {
		t.Errorf("expected [second, first], got %v", order)
	}
}

func TestModerator_NoParticipants(t *testing.T) {
	mod := NewModerator("empty")
	_, err := mod.Run(context.Background(), "topic")
	if err == nil {
		t.Fatal("expected error with no participants")
	}
}

func TestModerator_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := FuncParticipant("agent", func(_ context.Context, _ []Envelope) (string, error) {
		return "should not reach", nil
	})

	mod := NewModerator("test", WithMaxRounds(5))
	mod.AddParticipant(p)

	_, err := mod.Run(ctx, "topic")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestAsAgent(t *testing.T) {
	p := FuncParticipant("echo", func(_ context.Context, incoming []Envelope) (string, error) {
		return "reply", nil
	})

	mod := NewModerator("test-mod", WithMaxRounds(1))
	mod.AddParticipant(p)

	a := AsAgent(mod)
	if a.Name() != "test-mod" {
		t.Errorf("expected name test-mod, got %s", a.Name())
	}

	result, err := a.Run(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) < 2 {
		t.Errorf("expected at least 2 messages (seed + reply), got %d", len(result))
	}
}
