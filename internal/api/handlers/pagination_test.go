package handlers

import "testing"

func TestClampLimit(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 0},               // left to caller's own default handling
		{50, 50},             // typical page size, untouched
		{MaxPageLimit, MaxPageLimit},
		{MaxPageLimit + 1, MaxPageLimit}, // upper bound enforced
		{1_000_000, MaxPageLimit},        // abusive value capped
		{-5, -5},                         // negatives left to caller (n>0 guards upstream)
	}
	for _, c := range cases {
		if got := clampLimit(c.in); got != c.want {
			t.Errorf("clampLimit(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
