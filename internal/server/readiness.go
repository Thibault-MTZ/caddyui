// SPDX-License-Identifier: Apache-2.0

package server

// requiredReadinessStepCount is how many foundations the dashboard banner
// counts towards "ready". Kept next to requiredReadinessSteps so the
// denominator and the numerator cannot drift apart.
const requiredReadinessStepCount = 3

// requiredReadinessSteps returns the number of completed operating
// foundations shown by the onboarding and dashboard readiness workflows.
// Identity is always complete for an authenticated user. DNS automation is
// intentionally excluded because it is useful for DNS-01 and record
// management, but it is not required to operate an ordinary proxy host.
//
// v2.64.2 (issue #138): two-factor auth on the administrator's own account is
// excluded for the same reason. The onboarding step for it is labelled
// "Recommended", yet it used to be counted among the required foundations —
// so an administrator who had deliberately decided against TOTP was told
// "3 of 4 required / Continue setup" forever, with no way to finish. A
// recommendation that permanently blocks a complete state is just a nag.
// The step is still shown, and the dashboard still raises the site-wide
// policy under "Needs attention"; neither claims the install is unfinished.
func requiredReadinessSteps(serverConnected, servicePublished bool) int {
	completed := 1
	if serverConnected {
		completed++
	}
	if servicePublished {
		completed++
	}
	return completed
}

// readinessIncomplete reports whether the dashboard should still show the
// readiness banner — that is, whether a required foundation is missing.
func readinessIncomplete(serverConnected, servicePublished bool) bool {
	return requiredReadinessSteps(serverConnected, servicePublished) < requiredReadinessStepCount
}
