package observe

import "math/rand/v2"

// Sampler decides whether a trace should be recorded.
type Sampler interface {
	// ShouldSample returns true if the trace should be recorded.
	ShouldSample(traceID string) bool
}

// AlwaysSample records every trace.
type AlwaysSample struct{}

func (AlwaysSample) ShouldSample(string) bool { return true }

// NeverSample drops every trace.
type NeverSample struct{}

func (NeverSample) ShouldSample(string) bool { return false }

// RatioSampler samples a fraction of traces (0.0 to 1.0).
// A ratio of 0.1 means 10% of traces are recorded.
type RatioSampler struct {
	ratio float64
}

// NewRatioSampler creates a sampler that records the given fraction of traces.
func NewRatioSampler(ratio float64) *RatioSampler {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return &RatioSampler{ratio: ratio}
}

// ShouldSample returns true with probability equal to the configured ratio.
func (s *RatioSampler) ShouldSample(string) bool {
	return rand.Float64() < s.ratio
}
