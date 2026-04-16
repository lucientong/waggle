package observe

import (
	"testing"
)

func TestAlwaysSample(t *testing.T) {
	s := AlwaysSample{}
	for i := 0; i < 100; i++ {
		if !s.ShouldSample("trace-id") {
			t.Fatal("AlwaysSample should always return true")
		}
	}
}

func TestNeverSample(t *testing.T) {
	s := NeverSample{}
	for i := 0; i < 100; i++ {
		if s.ShouldSample("trace-id") {
			t.Fatal("NeverSample should always return false")
		}
	}
}

func TestRatioSampler_Zero(t *testing.T) {
	s := NewRatioSampler(0)
	for i := 0; i < 100; i++ {
		if s.ShouldSample("id") {
			t.Fatal("ratio 0 should never sample")
		}
	}
}

func TestRatioSampler_One(t *testing.T) {
	s := NewRatioSampler(1.0)
	for i := 0; i < 100; i++ {
		if !s.ShouldSample("id") {
			t.Fatal("ratio 1.0 should always sample")
		}
	}
}

func TestRatioSampler_Half(t *testing.T) {
	s := NewRatioSampler(0.5)
	sampled := 0
	total := 10000
	for i := 0; i < total; i++ {
		if s.ShouldSample("id") {
			sampled++
		}
	}
	// With 10000 trials at 0.5, expect ~5000. Allow wide margin.
	ratio := float64(sampled) / float64(total)
	if ratio < 0.4 || ratio > 0.6 {
		t.Errorf("expected ~50%% sampling, got %.1f%% (%d/%d)", ratio*100, sampled, total)
	}
}

func TestRatioSampler_Clamp(t *testing.T) {
	s1 := NewRatioSampler(-0.5)
	if s1.ratio != 0 {
		t.Errorf("negative ratio should clamp to 0, got %f", s1.ratio)
	}
	s2 := NewRatioSampler(2.0)
	if s2.ratio != 1 {
		t.Errorf("ratio > 1 should clamp to 1, got %f", s2.ratio)
	}
}
