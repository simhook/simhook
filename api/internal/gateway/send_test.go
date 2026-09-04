package gateway

import (
	"testing"
	"time"
)

func TestNormalizeRecipient(t *testing.T) {
	cases := map[string]string{
		"+1 (415) 555-0123": "+14155550123",
		"0044 20 7946 0958": "+442079460958",
		"  5550123 ":        "5550123",
		"+1.415.555.0123":   "+14155550123",
	}
	for in, want := range cases {
		got, ok := NormalizeRecipient(in)
		if !ok || got != want {
			t.Errorf("%q: got %q ok=%v, want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "abc", "+", "12", "+1 415 555 0123 ext 4", "1234567890123456"} {
		if _, ok := NormalizeRecipient(bad); ok {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestPlanWaves(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	waves := planWaves(95, 40, 5*time.Second, base)
	if len(waves) != 3 {
		t.Fatalf("want 3 waves, got %d", len(waves))
	}
	if waves[0].start != 0 || waves[0].end != 40 || !waves[0].due.Equal(base) {
		t.Errorf("wave 0: %+v", waves[0])
	}
	if waves[1].start != 40 || waves[1].end != 80 || !waves[1].due.Equal(base.Add(200*time.Second)) {
		t.Errorf("wave 1: %+v", waves[1])
	}
	if waves[2].start != 80 || waves[2].end != 95 || !waves[2].due.Equal(base.Add(400*time.Second)) {
		t.Errorf("wave 2: %+v", waves[2])
	}
	if got := planWaves(1, 0, 0, base); len(got) != 1 || got[0].end != 1 {
		t.Errorf("degenerate: %+v", got)
	}
	if got := planWaves(0, 40, time.Second, base); len(got) != 0 {
		t.Errorf("zero recipients should produce no waves: %+v", got)
	}
}
