package core

import "testing"

func TestTUNPermissionUsesEffectiveNetAdmin(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{"CapPrm:\t0000000000001000\nCapEff:\t0000000000000000\n", false},
		{"CapEff:\t0000000000001000\n", true},
		{"CapEff:\t0000000000002000\n", false},
	} {
		if got := hasNetAdmin(tc.status); got != tc.want {
			t.Errorf("hasNetAdmin(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}
