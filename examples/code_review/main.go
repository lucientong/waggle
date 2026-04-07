// Example: Code Review Pipeline
//
// This example demonstrates a multi-stage code review workflow:
//
//  1. Fetch: Simulate fetching PR code content
//  2. Analyze: Static analysis (find bugs, style issues)
//  3. Summarize: Produce a concise review summary
//  4. Review: Final quality gate (pass/fail decision)
//
// The pipeline uses Chain4 for type-safe serial composition,
// WithRetry for resilience on the LLM stages,
// and WithCache to avoid re-analyzing unchanged code.
//
// Run:
//
//	go run ./examples/code_review/
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
)

// CodeContent represents the source code to be reviewed.
type CodeContent struct {
	PR       string
	Language string
	Code     string
}

// AnalysisResult holds the findings from static analysis.
type AnalysisResult struct {
	Content  CodeContent
	Issues   []string
	Severity string // "none", "low", "medium", "high"
}

// ReviewResult is the final output of the review pipeline.
type ReviewResult struct {
	PR       string
	Summary  string
	Decision string // "approve", "request_changes", "reject"
	Score    int    // 0-100
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx := context.Background()

	// ---- Stage 1: Fetch PR content ----------------------------------------
	fetchPR := agent.Func[string, CodeContent]("fetch-pr", func(_ context.Context, prURL string) (CodeContent, error) {
		slog.Info("fetching PR", "url", prURL)
		// Simulate fetching; in production this calls the GitHub API.
		return CodeContent{
			PR:       prURL,
			Language: "Go",
			Code: `
func divide(a, b int) int {
    return a / b  // BUG: no zero-division check
}

func ProcessItems(items []string) {
    for i := 0; i <= len(items); i++ {  // BUG: off-by-one, should be <
        fmt.Println(items[i])
    }
}
`,
		}, nil
	})

	// ---- Stage 2: Static analysis -----------------------------------------
	analyze := agent.Func[CodeContent, AnalysisResult]("static-analysis", func(_ context.Context, content CodeContent) (AnalysisResult, error) {
		slog.Info("analyzing code", "language", content.Language)
		issues := []string{}

		if strings.Contains(content.Code, "/ b") && !strings.Contains(content.Code, "b == 0") {
			issues = append(issues, "potential zero-division: check that divisor is non-zero")
		}
		if strings.Contains(content.Code, "<= len(") {
			issues = append(issues, "off-by-one error: loop bound should use < not <=")
		}

		severity := "none"
		if len(issues) > 0 {
			severity = "high"
		}
		return AnalysisResult{Content: content, Issues: issues, Severity: severity}, nil
	})

	// ---- Stage 3: Summarize findings --------------------------------------
	summarize := agent.Func[AnalysisResult, string]("summarize", func(_ context.Context, result AnalysisResult) (string, error) {
		slog.Info("summarizing", "issues", len(result.Issues), "severity", result.Severity)
		if len(result.Issues) == 0 {
			return "No issues found. Code looks good!", nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d issue(s) with %s severity:\n", len(result.Issues), result.Severity)
		for i, issue := range result.Issues {
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, issue)
		}
		return sb.String(), nil
	})

	// ---- Stage 4: Final review decision -----------------------------------
	review := agent.Func[string, ReviewResult]("review-gate", func(_ context.Context, summary string) (ReviewResult, error) {
		slog.Info("making review decision")
		decision := "approve"
		score := 95
		if strings.Contains(summary, "high severity") || strings.Contains(summary, "issue(s)") {
			decision = "request_changes"
			score = 45
		}
		return ReviewResult{
			Summary:  summary,
			Decision: decision,
			Score:    score,
		}, nil
	})

	// ---- Assemble the pipeline --------------------------------------------
	// Chain4: fetch -> analyze -> summarize -> review
	// WithRetry on analyze (LLM calls can be flaky)
	// WithCache on fetch (avoid re-fetching the same PR)
	pipeline := agent.Chain4(
		agent.WithCache(fetchPR, func(url string) string { return url }),
		agent.WithRetry(analyze, agent.WithMaxAttempts(3), agent.WithBaseDelay(50*time.Millisecond)),
		summarize,
		review,
	)

	// ---- Run the pipeline -------------------------------------------------
	prURL := "https://github.com/lucientong/waggle/pull/42"
	fmt.Printf("Running code review pipeline for: %s\n\n", prURL)

	result, err := pipeline.Run(ctx, prURL)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		os.Exit(1)
	}

	// ---- Print results ----------------------------------------------------
	fmt.Printf("=== Code Review Result ===\n")
	fmt.Printf("Decision : %s\n", result.Decision)
	fmt.Printf("Score    : %d/100\n", result.Score)
	fmt.Printf("Summary  :\n%s\n", result.Summary)

	// Run again — fetch should be served from cache.
	fmt.Printf("\n--- Second run (fetch from cache) ---\n")
	result2, _ := pipeline.Run(ctx, prURL)
	fmt.Printf("Cached decision: %s\n", result2.Decision)
}
