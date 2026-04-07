// Example: Proactive Agent with Layered Timer
//
// This example demonstrates a proactive messaging agent that decides
// whether to reach out to users based on a layered decision pipeline:
//
//  1. Timer Check (L0): Query a timer queue for pending triggers
//  2. Cheap Judge (L1): Use a lightweight model to decide if action is needed
//  3. Response Generator (L2): Use a more capable model to craft the message
//
// Patterns used:
//   - waggle.Router: route based on timer check result (trigger / skip)
//   - agent.Chain3: serial pipeline from check → judge → respond
//   - agent.Func: wrap decision logic as type-safe agents
//   - agent.WithTimeout: bound the decision pipeline
//
// This example simulates the pipeline without real LLM calls.
//
// Run:
//
//	go run ./examples/proactive_agent/
package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/waggle"
)

// TimerEntry represents a pending proactive trigger in the timer queue.
type TimerEntry struct {
	UserID    string
	TriggerAt time.Time
	Reason    string // e.g., "follow_up", "reminder", "check_in"
	Context   string // recent conversation context
}

// JudgeResult is the output of the L1 cheap judge.
type JudgeResult struct {
	Entry      TimerEntry
	ShouldAct  bool
	Reason     string
	Confidence float64 // 0.0 - 1.0
}

// ProactiveMessage is the final output of the pipeline.
type ProactiveMessage struct {
	UserID   string
	Message  string
	Channel  string // "im", "email", "push"
	Priority string // "low", "medium", "high"
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx := context.Background()

	// ---- L0: Timer Queue Check -------------------------------------------
	// Simulate checking a Redis sorted set / SQLite timer queue.
	checkTimer := agent.Func[string, []TimerEntry]("timer-check", func(_ context.Context, userID string) ([]TimerEntry, error) {
		slog.Info("checking timer queue", "user", userID)

		// Simulated timer entries (in production, query Redis ZRANGEBYSCORE).
		now := time.Now()
		entries := []TimerEntry{
			{
				UserID:    userID,
				TriggerAt: now.Add(-5 * time.Minute), // 5 min overdue
				Reason:    "follow_up",
				Context:   "User asked about project deadline yesterday",
			},
			{
				UserID:    userID,
				TriggerAt: now.Add(30 * time.Minute), // not yet due
				Reason:    "reminder",
				Context:   "Team meeting at 3pm",
			},
		}

		// Filter only entries that are due.
		var due []TimerEntry
		for _, e := range entries {
			if e.TriggerAt.Before(now) || e.TriggerAt.Equal(now) {
				due = append(due, e)
			}
		}
		slog.Info("timer check complete", "total", len(entries), "due", len(due))
		return due, nil
	})

	// ---- L1: Cheap Judge -------------------------------------------------
	// A lightweight model decides if the trigger is worth acting on.
	// In production, use a small/fast LLM (e.g., GPT-4o-mini, Gemini Flash).
	cheapJudge := agent.Func[TimerEntry, JudgeResult]("cheap-judge", func(_ context.Context, entry TimerEntry) (JudgeResult, error) {
		slog.Info("L1 judging", "user", entry.UserID, "reason", entry.Reason)

		// Simulate lightweight model inference.
		shouldAct := true
		confidence := 0.7 + rand.Float64()*0.3 //nolint:gosec

		// Simple heuristic: skip if context is too vague.
		if len(entry.Context) < 10 {
			shouldAct = false
			confidence = 0.2
		}

		return JudgeResult{
			Entry:      entry,
			ShouldAct:  shouldAct,
			Reason:     fmt.Sprintf("Timer '%s' is due. Context: %s", entry.Reason, entry.Context),
			Confidence: confidence,
		}, nil
	})

	// ---- L2: Response Generator ------------------------------------------
	// The capable model generates the actual proactive message.
	// In production, use GPT-4o, Claude, etc.
	generateResponse := agent.Func[JudgeResult, ProactiveMessage]("response-gen", func(_ context.Context, judge JudgeResult) (ProactiveMessage, error) {
		slog.Info("L2 generating response", "user", judge.Entry.UserID, "confidence", judge.Confidence)

		// Simulate response generation based on trigger reason.
		var msg string
		priority := "medium"
		channel := "im"

		switch judge.Entry.Reason {
		case "follow_up":
			msg = fmt.Sprintf("Hi! Just following up on our conversation about the project deadline. %s — would you like me to help with anything?",
				judge.Entry.Context)
			priority = "high"
		case "reminder":
			msg = fmt.Sprintf("Friendly reminder: %s. Would you like me to prepare anything?", judge.Entry.Context)
			priority = "medium"
		case "check_in":
			msg = fmt.Sprintf("Hey! Checking in — %s. How's everything going?", judge.Entry.Context)
			priority = "low"
		default:
			msg = fmt.Sprintf("Hi! I noticed something that might need your attention: %s", judge.Entry.Context)
		}

		return ProactiveMessage{
			UserID:   judge.Entry.UserID,
			Message:  msg,
			Channel:  channel,
			Priority: priority,
		}, nil
	})

	// ---- Router: Act or Skip based on judge result -----------------------
	actAgent := agent.Func[JudgeResult, ProactiveMessage]("act", func(ctx context.Context, j JudgeResult) (ProactiveMessage, error) {
		return generateResponse.Run(ctx, j)
	})

	skipAgent := agent.Func[JudgeResult, ProactiveMessage]("skip", func(_ context.Context, j JudgeResult) (ProactiveMessage, error) {
		slog.Info("skipping — judge says no action needed", "user", j.Entry.UserID)
		return ProactiveMessage{
			UserID:  j.Entry.UserID,
			Message: "(skipped — no action needed)",
		}, nil
	})

	decisionRouter := waggle.Router(
		"act-or-skip",
		func(_ context.Context, j JudgeResult) (string, error) {
			if j.ShouldAct && j.Confidence > 0.5 {
				return "act", nil
			}
			return "skip", nil
		},
		map[string]agent.Agent[JudgeResult, ProactiveMessage]{
			"act":  actAgent,
			"skip": skipAgent,
		},
	)

	// ---- Full Pipeline: Timer → Judge → Route ----------------------------
	pipeline := agent.Chain2(cheapJudge, decisionRouter)

	// Wrap with a timeout for the entire decision pipeline.
	boundedPipeline := agent.WithTimeout(pipeline, 10*time.Second)

	// ---- Run the demo ----------------------------------------------------
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Proactive Agent — Layered Timer Demo")
	fmt.Println("═══════════════════════════════════════════════════════════")

	userID := "user-42"

	// Step 1: Check timer queue.
	entries, err := checkTimer.Run(ctx, userID)
	if err != nil {
		slog.Error("timer check failed", "error", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("\nNo pending triggers. Agent stays quiet. 🤫")
		return
	}

	// Step 2: Process each due entry through the layered pipeline.
	fmt.Printf("\nFound %d due trigger(s) for %s:\n\n", len(entries), userID)

	for i, entry := range entries {
		fmt.Printf("─── Trigger %d: %s ───\n", i+1, entry.Reason)
		fmt.Printf("  Due at  : %s\n", entry.TriggerAt.Format(time.RFC3339))
		fmt.Printf("  Context : %s\n", entry.Context)

		result, err := boundedPipeline.Run(ctx, entry)
		if err != nil {
			slog.Error("pipeline failed", "entry", entry.Reason, "error", err)
			continue
		}

		fmt.Printf("  Decision: %s\n", resultDecision(result))
		if result.Message != "(skipped — no action needed)" {
			fmt.Printf("  Channel : %s\n", result.Channel)
			fmt.Printf("  Priority: %s\n", result.Priority)
			fmt.Printf("  Message :\n    %s\n", result.Message)
		}
		fmt.Println()
	}

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Pipeline complete. All triggers processed.")
	fmt.Println("═══════════════════════════════════════════════════════════")
}

func resultDecision(m ProactiveMessage) string {
	if strings.Contains(m.Message, "skipped") {
		return "SKIP"
	}
	return "ACT → send message"
}
