package format

import (
	"math"
	"testing"
)

func TestSignedPct(t *testing.T) {
	cases := []struct {
		v    float64
		want string
	}{
		{5.43, "+5.4%"},
		{0, "+0.0%"},
		{-65.78, "-65.8%"},
		{math.NaN(), "—"},
	}
	for _, c := range cases {
		if got := SignedPct(c.v); got != c.want {
			t.Errorf("SignedPct(%v) = %q, want %q", c.v, got, c.want)
		}
	}
}
