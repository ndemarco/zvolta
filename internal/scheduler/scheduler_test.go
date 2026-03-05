package scheduler

import (
	"testing"
	"time"

	"github.com/ndemarco/zvolta/internal/policy"
	"github.com/ndemarco/zvolta/internal/snapshot"
)

func testPolicy() *policy.Policy {
	return &policy.Policy{
		Dataset:        "tank/data",
		AutoSnap:       true,
		AutoPrune:      true,
		FrequentCount:  12,
		FrequentPeriod: 5,
		HourlyCount:    24,
		DailyCount:     30,
		WeeklyCount:    4,
		MonthlyCount:   12,
		YearlyCount:    3,
	}
}

func TestDueAllTiersOnFreshStart(t *testing.T) {
	s := &Schedule{}
	p := testPolicy()
	now := time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC) // Thursday

	// No previous snapshots — all enabled tiers should be due
	lastSnap := make(map[snapshot.Tier]time.Time)
	due := s.Due(p, now, lastSnap)

	if len(due) != 6 {
		t.Errorf("expected 6 due tiers on fresh start, got %d: %v", len(due), due)
	}
}

func TestDueHourlyNotDueWithinSameHour(t *testing.T) {
	s := &Schedule{}
	p := testPolicy()
	now := time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC)

	lastSnap := map[snapshot.Tier]time.Time{
		snapshot.TierHourly: time.Date(2026, 3, 5, 14, 0, 0, 0, time.UTC), // taken this hour
	}

	due := s.Due(p, now, lastSnap)
	for _, tier := range due {
		if tier == snapshot.TierHourly {
			t.Error("hourly should not be due — already taken this hour")
		}
	}
}

func TestDueHourlyDueAfterHourBoundary(t *testing.T) {
	s := &Schedule{}
	p := testPolicy()
	now := time.Date(2026, 3, 5, 15, 1, 0, 0, time.UTC) // 15:01

	lastSnap := map[snapshot.Tier]time.Time{
		snapshot.TierHourly: time.Date(2026, 3, 5, 14, 0, 0, 0, time.UTC), // taken at 14:00
	}

	due := s.Due(p, now, lastSnap)
	found := false
	for _, tier := range due {
		if tier == snapshot.TierHourly {
			found = true
		}
	}
	if !found {
		t.Error("hourly should be due — hour boundary crossed")
	}
}

func TestDueFrequentPeriod(t *testing.T) {
	s := &Schedule{}
	p := testPolicy()
	now := time.Date(2026, 3, 5, 14, 10, 0, 0, time.UTC) // 14:10

	// Last frequent at 14:05 — next due at 14:10
	lastSnap := map[snapshot.Tier]time.Time{
		snapshot.TierFrequent: time.Date(2026, 3, 5, 14, 5, 0, 0, time.UTC),
	}

	due := s.Due(p, now, lastSnap)
	found := false
	for _, tier := range due {
		if tier == snapshot.TierFrequent {
			found = true
		}
	}
	if !found {
		t.Error("frequent should be due at 14:10 with 5-min period")
	}
}

func TestDueFrequentNotDueYet(t *testing.T) {
	s := &Schedule{}
	p := testPolicy()
	now := time.Date(2026, 3, 5, 14, 7, 0, 0, time.UTC) // 14:07

	// Last frequent at 14:05 — next boundary at 14:10
	lastSnap := map[snapshot.Tier]time.Time{
		snapshot.TierFrequent: time.Date(2026, 3, 5, 14, 5, 0, 0, time.UTC),
	}

	due := s.Due(p, now, lastSnap)
	for _, tier := range due {
		if tier == snapshot.TierFrequent {
			t.Error("frequent should not be due at 14:07 — boundary is 14:10")
		}
	}
}

func TestDueWithOffset(t *testing.T) {
	s := &Schedule{Offset: 5 * time.Minute}
	p := &policy.Policy{
		Dataset:     "tank/data",
		AutoSnap:    true,
		HourlyCount: 24,
	}

	// At 14:04 with +5m offset, the effective boundary is 13:05 (previous hour + 5m offset).
	// Actually: adjusted time = 14:04 - 5m = 13:59, so previous hourly boundary = 13:00.
	// Real boundary = 13:00 + offset = effectively, the boundary in real time is at 14:05.
	// So at 14:04, we're still before the offset boundary.
	now := time.Date(2026, 3, 5, 14, 4, 0, 0, time.UTC)
	lastSnap := map[snapshot.Tier]time.Time{
		snapshot.TierHourly: time.Date(2026, 3, 5, 13, 5, 0, 0, time.UTC),
	}

	due := s.Due(p, now, lastSnap)
	for _, tier := range due {
		if tier == snapshot.TierHourly {
			t.Error("hourly should not be due at 14:04 with +5m offset")
		}
	}

	// At 14:06 with +5m offset, adjusted = 14:01, boundary = 14:00, which is after 13:05
	now = time.Date(2026, 3, 5, 14, 6, 0, 0, time.UTC)
	due = s.Due(p, now, lastSnap)
	found := false
	for _, tier := range due {
		if tier == snapshot.TierHourly {
			found = true
		}
	}
	if !found {
		t.Error("hourly should be due at 14:06 with +5m offset")
	}
}

func TestDueWeeklyBoundary(t *testing.T) {
	s := &Schedule{}
	p := &policy.Policy{
		Dataset:     "tank/data",
		AutoSnap:    true,
		WeeklyCount: 4,
	}

	// Sunday March 8 2026 is a Sunday
	now := time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC)
	lastSnap := map[snapshot.Tier]time.Time{
		snapshot.TierWeekly: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), // last Sunday
	}

	due := s.Due(p, now, lastSnap)
	found := false
	for _, tier := range due {
		if tier == snapshot.TierWeekly {
			found = true
		}
	}
	if !found {
		t.Error("weekly should be due on new Sunday")
	}
}

func TestDueDisabledTier(t *testing.T) {
	s := &Schedule{}
	p := &policy.Policy{
		Dataset:     "tank/data",
		AutoSnap:    true,
		HourlyCount: 24,
		// daily is 0 — disabled
	}

	now := time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC)
	due := s.Due(p, now, make(map[snapshot.Tier]time.Time))

	for _, tier := range due {
		if tier == snapshot.TierDaily {
			t.Error("daily should not be due — count is 0")
		}
	}
}

func TestNextTick(t *testing.T) {
	now := time.Date(2026, 3, 5, 14, 30, 45, 0, time.UTC)
	next := NextTick(now)
	want := time.Date(2026, 3, 5, 14, 31, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextTick = %v, want %v", next, want)
	}
}
