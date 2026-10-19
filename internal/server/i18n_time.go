// SPDX-License-Identifier: Apache-2.0
package server

import "time"

func relativeTimeFunc(ctx any, value time.Time) string {
	return formatRelativeTime(ctx, value, time.Now())
}

// formatRelativeTime shares the existing compact display thresholds while
// looking up whole phrases per request. No process-wide locale is changed.
func formatRelativeTime(ctx any, value, now time.Time) string {
	if value.IsZero() {
		return tFunc(ctx, "time.relative.never")
	}
	delta := now.Sub(value)
	direction := "past"
	if delta < 0 {
		direction = "future"
		delta = -delta
	}
	if delta >= 30*24*time.Hour {
		date := value.In(activeLocation()).Format("2006-01-02")
		if direction == "future" {
			return tFunc(ctx, "time.relative.future_date", "date", date)
		}
		return date
	}
	unit := "seconds"
	count := int(delta.Seconds())
	switch {
	case delta >= 24*time.Hour:
		unit = "days"
		count = int(delta.Hours()) / 24
	case delta >= time.Hour:
		unit = "hours"
		count = int(delta.Hours())
	case delta >= time.Minute:
		unit = "minutes"
		count = int(delta.Minutes())
	}
	return tFunc(ctx, "time.relative."+direction+"_"+unit, "count", count)
}

func humanDurationLocalized(value time.Duration, lang string) string {
	if value < 0 {
		value = -value
	}
	unit := "minute"
	count := int(value.Minutes())
	switch {
	case value >= 24*time.Hour:
		unit = "day"
		count = int(value.Hours()) / 24
	case value >= time.Hour:
		unit = "hour"
		count = int(value.Hours())
	}
	if count < 1 {
		count = 1
	}
	form := "other"
	if count == 1 {
		form = "one"
	}
	return tFunc(lang, "time.duration."+unit+"."+form, "count", count)
}
