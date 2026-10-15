// SPDX-License-Identifier: Apache-2.0

package server

import "testing"

func TestRequiredReadinessSteps(t *testing.T) {
	tests := []struct {
		name                          string
		serverConnected, serviceReady bool
		want                          int
	}{
		{name: "identity only", want: 1},
		{name: "connected", serverConnected: true, want: 2},
		{name: "ready", serverConnected: true, serviceReady: true, want: 3},
		{name: "service without a connected server", serviceReady: true, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requiredReadinessSteps(tt.serverConnected, tt.serviceReady)
			if got != tt.want {
				t.Fatalf("requiredReadinessSteps() = %d, want %d", got, tt.want)
			}
		})
	}
}

// v2.64.2 (issue #138): the administrator's own TOTP is presented as
// "Recommended" on the onboarding page, but used to be counted among the
// required foundations — so an admin who had decided against it saw
// "3 of 4 required / Continue setup" with nothing left to complete. The
// banner must be able to reach a finished state without it.
func TestReadinessCompletesWithoutTwoFactor(t *testing.T) {
	if got := requiredReadinessSteps(true, true); got != requiredReadinessStepCount {
		t.Fatalf("a connected server with a published service should be complete: %d of %d", got, requiredReadinessStepCount)
	}
	if readinessIncomplete(true, true) {
		t.Error("banner still showing with every required foundation in place")
	}
}

func TestReadinessStillFlagsGenuinelyIncompleteSetups(t *testing.T) {
	for name, tc := range map[string]struct{ connected, published bool }{
		"nothing configured":   {false, false},
		"no Caddy connection":  {false, true},
		"no published service": {true, false},
	} {
		t.Run(name, func(t *testing.T) {
			if !readinessIncomplete(tc.connected, tc.published) {
				t.Error("a required foundation is missing, so the banner must still show")
			}
		})
	}
}

// The denominator in the UI string and the numerator here have to agree, or
// the banner reads "3 of 4" forever.
func TestReadinessCountMatchesItsMaximum(t *testing.T) {
	if got := requiredReadinessSteps(true, true); got != requiredReadinessStepCount {
		t.Fatalf("maximum completed = %d but the banner says %d required", got, requiredReadinessStepCount)
	}
}
