package ratelimit

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestKeyed(t *testing.T) {
	k := NewKeyed(rate.Every(time.Hour), 2)
	for i := 0; i < 2; i++ {
		if !k.Allow("a") {
			t.Fatal("the burst should pass")
		}
	}
	if k.Allow("a") {
		t.Fatal("the third call within the hour should be refused")
	}
	if !k.Allow("b") {
		t.Fatal("keys are independent")
	}
}
