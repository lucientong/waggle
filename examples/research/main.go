// Example: Research Assistant
//
// This example demonstrates a parallel research workflow that gathers information
// from multiple sources concurrently, then synthesizes the findings:
//
//  1. Search: Parallel search across three simulated "databases"
//  2. Extract: Extract key facts from each search result  (via Race — fastest wins)
//  3. Synthesize: Combine all extracted facts into a coherent report
//
// Patterns used:
//   - waggle.Parallel: fan out search to three sources simultaneously
//   - waggle.Race: use the fastest extraction service
//   - agent.Chain2: pipe synthesis after parallel collection
//
// Run:
//
//	go run ./examples/research/
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/waggle"
)

// SearchResult holds raw text from a single source.
type SearchResult struct {
	Source string
	Text   string
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx := context.Background()
	query := "Go generics best practices 2024"

	fmt.Printf("Research query: %q\n\n", query)

	// ---- Parallel search across 3 sources ---------------------------------
	wikipedia := agent.Func[string, SearchResult]("wikipedia", func(_ context.Context, q string) (SearchResult, error) {
		slog.Info("searching wikipedia", "query", q)
		time.Sleep(30 * time.Millisecond) // simulate latency
		return SearchResult{Source: "Wikipedia", Text: fmt.Sprintf("Wikipedia article about: %s. Go generics use type parameters [T any] syntax.", q)}, nil
	})

	arxiv := agent.Func[string, SearchResult]("arxiv", func(_ context.Context, q string) (SearchResult, error) {
		slog.Info("searching arxiv", "query", q)
		time.Sleep(50 * time.Millisecond)
		return SearchResult{Source: "arXiv", Text: fmt.Sprintf("Research paper on %s: type inference, constraints, and performance benchmarks.", q)}, nil
	})

	stackoverflow := agent.Func[string, SearchResult]("stackoverflow", func(_ context.Context, q string) (SearchResult, error) {
		slog.Info("searching stackoverflow", "query", q)
		time.Sleep(20 * time.Millisecond)
		return SearchResult{Source: "StackOverflow", Text: fmt.Sprintf("Top answers for '%s': use constraints wisely, avoid over-engineering.", q)}, nil
	})

	// All three search in parallel.
	searchAll := waggle.Parallel("search-all", wikipedia, arxiv, stackoverflow)

	// ---- Race: fastest extraction service ---------------------------------
	extractFast := agent.Func[SearchResult, string]("extract-fast", func(_ context.Context, sr SearchResult) (string, error) {
		slog.Info("extracting (fast)", "source", sr.Source)
		return fmt.Sprintf("[fast] Key points from %s: %s", sr.Source, sr.Text[:min(len(sr.Text), 60)]), nil
	})

	extractRobust := agent.Func[SearchResult, string]("extract-robust", func(ctx context.Context, sr SearchResult) (string, error) {
		slog.Info("extracting (robust)", "source", sr.Source)
		select {
		case <-time.After(100 * time.Millisecond): // slower but more thorough
		case <-ctx.Done():
			return "", ctx.Err()
		}
		return fmt.Sprintf("[robust] Key points from %s: %s", sr.Source, sr.Text[:min(len(sr.Text), 80)]), nil
	})

	// ---- Synthesize all results -------------------------------------------
	// The parallel results come back as waggle.ParallelResults[SearchResult].
	// We use a custom agent to extract and synthesize them.
	synthesize := agent.Func[waggle.ParallelResults[SearchResult], string](
		"synthesize",
		func(ctx context.Context, results waggle.ParallelResults[SearchResult]) (string, error) {
			slog.Info("synthesizing", "sources", len(results.Results))

			var lines []string
			lines = append(lines, fmt.Sprintf("Research Report: %q\n", query))
			lines = append(lines, strings.Repeat("=", 50))

			for i, sr := range results.Results {
				if results.Errors[i] != nil {
					lines = append(lines, fmt.Sprintf("  [%d] ERROR: %v", i, results.Errors[i]))
					continue
				}

				// Apply race-based extraction to each result.
				extractor := waggle.Race("extract", extractFast, extractRobust)
				extracted, err := extractor.Run(ctx, sr)
				if err != nil {
					lines = append(lines, fmt.Sprintf("  [%d] extraction failed: %v", i, err))
					continue
				}
				lines = append(lines, fmt.Sprintf("  • %s", extracted))
			}

			lines = append(lines, strings.Repeat("=", 50))
			lines = append(lines, "Synthesis: All sources agree that Go generics improve type safety")
			lines = append(lines, "and code reuse while requiring careful constraint design.")

			return strings.Join(lines, "\n"), nil
		},
	)

	// ---- Chain Parallel -> Synthesize -------------------------------------
	pipeline := agent.Chain2(searchAll, synthesize)

	result, err := pipeline.Run(ctx, query)
	if err != nil {
		slog.Error("research pipeline failed", "error", err)
		os.Exit(1)
	}

	fmt.Println(result)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
