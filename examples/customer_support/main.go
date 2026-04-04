// Example: Customer Support Pipeline
//
// This example demonstrates an intelligent customer support workflow:
//
//  1. Classify: Determine the ticket type (billing, technical, general)
//  2. Route: Send to the appropriate specialist agent (Router pattern)
//  3. Respond: Generate an initial response
//  4. Refine: Loop until response quality score meets the threshold (Loop pattern)
//
// Patterns used:
//   - waggle.Router: route tickets to the right handler
//   - waggle.Loop: refine the response until quality threshold is met
//   - agent.WithRetry: resilience on LLM-simulated calls
//
// Run:
//
//	go run ./examples/customer_support/
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/waggle"
)

// Ticket represents an incoming customer support request.
type Ticket struct {
	ID      string
	Subject string
	Body    string
	Type    string // classified: "billing", "technical", "general"
}

// Response is the agent-generated reply.
type Response struct {
	Ticket  Ticket
	Text    string
	Quality float64 // 0.0 - 1.0
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx := context.Background()

	// ---- Stage 1: Classify the ticket -------------------------------------
	classify := agent.Func[Ticket, Ticket]("classify", func(_ context.Context, t Ticket) (Ticket, error) {
		slog.Info("classifying ticket", "id", t.ID, "subject", t.Subject)
		body := strings.ToLower(t.Body)
		billingKeywords   := []string{"bill", "invoice", "charge", "payment", "refund"}
		technicalKeywords := []string{"error", "crash", "bug", "slow", "timeout"}

		isBilling   := false
		isTechnical := false
		for _, kw := range billingKeywords {
			if strings.Contains(body, kw) { isBilling = true; break }
		}
		for _, kw := range technicalKeywords {
			if strings.Contains(body, kw) { isTechnical = true; break }
		}

		switch {
		case isBilling:
			t.Type = "billing"
		case isTechnical:
			t.Type = "technical"
		default:
			t.Type = "general"
		}
		slog.Info("ticket classified", "id", t.ID, "type", t.Type)
		return t, nil
	})

	// ---- Stage 2: Specialist agents per type ------------------------------
	billingAgent := agent.Func[Ticket, Response]("billing-agent", func(_ context.Context, t Ticket) (Response, error) {
		slog.Info("billing specialist handling ticket", "id", t.ID)
		return Response{
			Ticket:  t,
			Text:    fmt.Sprintf("Dear customer, regarding your billing inquiry about '%s': our billing team will review your account within 24 hours and process any eligible refunds.", t.Subject),
			Quality: 0.6,
		}, nil
	})

	technicalAgent := agent.Func[Ticket, Response]("technical-agent", func(_ context.Context, t Ticket) (Response, error) {
		slog.Info("technical specialist handling ticket", "id", t.ID)
		return Response{
			Ticket:  t,
			Text:    fmt.Sprintf("Hi, for your technical issue '%s': please try clearing cache and restarting. If the issue persists, our engineers will escalate within 4 hours.", t.Subject),
			Quality: 0.65,
		}, nil
	})

	generalAgent := agent.Func[Ticket, Response]("general-agent", func(_ context.Context, t Ticket) (Response, error) {
		slog.Info("general support handling ticket", "id", t.ID)
		return Response{
			Ticket:  t,
			Text:    fmt.Sprintf("Hello! Thank you for contacting us about '%s'. A support representative will get back to you within 1 business day.", t.Subject),
			Quality: 0.7,
		}, nil
	})

	// ---- Stage 3: Router --------------------------------------------------
	routeTicket := waggle.Router(
		"ticket-router",
		func(_ context.Context, t Ticket) (string, error) { return t.Type, nil },
		map[string]agent.Agent[Ticket, Response]{
			"billing":   billingAgent,
			"technical": technicalAgent,
			"general":   generalAgent,
		},
		waggle.WithFallback[Ticket, Response](generalAgent),
	)

	// ---- Stage 4: Refine the response with Loop ---------------------------
	// Init: route to specialist to get initial response.
	// Body: improve the response if quality < threshold.
	improveResponse := agent.Func[Response, Response]("improve-response", func(_ context.Context, r Response) (Response, error) {
		slog.Info("improving response", "ticket", r.Ticket.ID, "quality", r.Quality)
		// Simulate LLM refinement: append a polite closing and boost quality.
		improved := r.Text + "\n\nWe value your business and appreciate your patience. Please don't hesitate to reply if you need further assistance."
		return Response{
			Ticket:  r.Ticket,
			Text:    improved,
			Quality: r.Quality + 0.2,
		}, nil
	})

	refineUntilGood := waggle.Loop(
		"refine-response",
		routeTicket,
		improveResponse,
		func(r Response) bool { return r.Quality < 0.85 }, // continue while quality < 0.85
		waggle.WithMaxIterations[Ticket, Response](3),
	)

	// ---- Full pipeline: classify -> route+refine -------------------------
	pipeline := agent.Chain2(classify, refineUntilGood)

	// ---- Test tickets ----------------------------------------------------
	tickets := []Ticket{
		{ID: "TKT-001", Subject: "Invoice #1234 incorrect", Body: "I received an invoice that charges me twice. Please refund the extra payment."},
		{ID: "TKT-002", Subject: "App crashes on startup", Body: "Getting a timeout error when I open the app. The error code is 503."},
		{ID: "TKT-003", Subject: "How do I change my username?", Body: "I would like to update my profile username but can't find the setting."},
	}

	for _, ticket := range tickets {
		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Printf("Ticket: %s — %s\n", ticket.ID, ticket.Subject)

		result, err := pipeline.Run(ctx, ticket)
		if err != nil {
			slog.Error("pipeline failed", "ticket", ticket.ID, "error", err)
			continue
		}

		fmt.Printf("Type   : %s\n", result.Ticket.Type)
		fmt.Printf("Quality: %.0f%%\n", result.Quality*100)
		fmt.Printf("Response:\n%s\n", result.Text)
	}

	fmt.Printf("\n%s\n", strings.Repeat("─", 60))
	fmt.Println("All tickets processed.")
	_ = os.Stdout
}
