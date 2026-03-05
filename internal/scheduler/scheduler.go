package scheduler

import (
	"time"

	"github.com/ndemarco/zvolta/internal/policy"
	"github.com/ndemarco/zvolta/internal/snapshot"
)

// Schedule determines which snapshot tiers are due for a dataset given its
// policy and the current time.
type Schedule struct {
	Offset time.Duration // global schedule offset from config
}

// Due returns the list of tiers that need a snapshot taken now, given the
// policy, current time, and the most recent snapshot time per tier.
func (s *Schedule) Due(p *policy.Policy, now time.Time, lastSnap map[snapshot.Tier]time.Time) []snapshot.Tier {
	var due []snapshot.Tier
	for _, tier := range snapshot.AllTiers() {
		if !p.IsEnabled(tier) {
			continue
		}
		boundary := s.previousBoundary(tier, p, now)
		last, exists := lastSnap[tier]
		if !exists || last.Before(boundary) {
			due = append(due, tier)
		}
	}
	return due
}

// previousBoundary returns the most recent clock-aligned boundary for a tier,
// adjusted by the global offset.
func (s *Schedule) previousBoundary(tier snapshot.Tier, p *policy.Policy, now time.Time) time.Time {
	// Apply offset: if offset is +5m, we shift the reference time back by 5m
	// so that boundaries appear 5 minutes later in real time.
	adjusted := now.UTC().Add(-s.Offset)

	switch tier {
	case snapshot.TierFrequent:
		period := time.Duration(p.FrequentPeriod) * time.Minute
		if period <= 0 {
			period = 5 * time.Minute
		}
		// Align to period within the hour
		minutesSinceHour := time.Duration(adjusted.Minute())*time.Minute + time.Duration(adjusted.Second())*time.Second
		periodsElapsed := minutesSinceHour / period
		hourStart := time.Date(adjusted.Year(), adjusted.Month(), adjusted.Day(), adjusted.Hour(), 0, 0, 0, time.UTC)
		return hourStart.Add(periodsElapsed * period)

	case snapshot.TierHourly:
		return time.Date(adjusted.Year(), adjusted.Month(), adjusted.Day(), adjusted.Hour(), 0, 0, 0, time.UTC)

	case snapshot.TierDaily:
		return time.Date(adjusted.Year(), adjusted.Month(), adjusted.Day(), 0, 0, 0, 0, time.UTC)

	case snapshot.TierWeekly:
		weekday := adjusted.Weekday()
		daysSinceSunday := int(weekday)
		sunday := adjusted.AddDate(0, 0, -daysSinceSunday)
		return time.Date(sunday.Year(), sunday.Month(), sunday.Day(), 0, 0, 0, 0, time.UTC)

	case snapshot.TierMonthly:
		return time.Date(adjusted.Year(), adjusted.Month(), 1, 0, 0, 0, 0, time.UTC)

	case snapshot.TierYearly:
		return time.Date(adjusted.Year(), 1, 1, 0, 0, 0, 0, time.UTC)

	default:
		return time.Time{}
	}
}

// NextTick returns the time of the next 1-minute tick boundary.
func NextTick(now time.Time) time.Time {
	return now.Truncate(time.Minute).Add(time.Minute)
}
