package snapshot

import (
	"fmt"
	"strings"
	"time"
)

// Tier represents a snapshot frequency tier.
type Tier string

const (
	TierFrequent Tier = "frequent"
	TierHourly   Tier = "hourly"
	TierDaily    Tier = "daily"
	TierWeekly   Tier = "weekly"
	TierMonthly  Tier = "monthly"
	TierYearly   Tier = "yearly"
)

// AllTiers returns all tiers in order from most to least frequent.
func AllTiers() []Tier {
	return []Tier{TierFrequent, TierHourly, TierDaily, TierWeekly, TierMonthly, TierYearly}
}

// ParseTier converts a string to a Tier, returning an error if invalid.
func ParseTier(s string) (Tier, error) {
	switch Tier(s) {
	case TierFrequent, TierHourly, TierDaily, TierWeekly, TierMonthly, TierYearly:
		return Tier(s), nil
	default:
		return "", fmt.Errorf("invalid tier: %q", s)
	}
}

// TimestampFormat is the ISO 8601 format used in snapshot names.
// Colons are replaced with hyphens because ZFS snapshot names cannot contain colons.
const TimestampFormat = "2006-01-02T15-04-05Z"

// FormatName builds a snapshot name from its components.
// Example: zvolta_hourly_2026-03-05T14-00-00Z
func FormatName(prefix string, tier Tier, t time.Time) string {
	return fmt.Sprintf("%s%s_%s", prefix, tier, t.UTC().Format(TimestampFormat))
}

// ParsedName holds the components extracted from a snapshot name.
type ParsedName struct {
	Prefix    string
	Tier      Tier
	Timestamp time.Time
}

// ParseName extracts components from a snapshot name. Returns an error if the
// name doesn't match the expected format or doesn't have the given prefix.
func ParseName(name, prefix string) (ParsedName, error) {
	if !strings.HasPrefix(name, prefix) {
		return ParsedName{}, fmt.Errorf("name %q does not start with prefix %q", name, prefix)
	}

	rest := name[len(prefix):]

	// Find tier: everything before the first underscore
	idx := strings.Index(rest, "_")
	if idx < 0 {
		return ParsedName{}, fmt.Errorf("name %q: no tier separator found", name)
	}

	tierStr := rest[:idx]
	tier, err := ParseTier(tierStr)
	if err != nil {
		return ParsedName{}, fmt.Errorf("name %q: %w", name, err)
	}

	tsStr := rest[idx+1:]
	t, err := time.Parse(TimestampFormat, tsStr)
	if err != nil {
		return ParsedName{}, fmt.Errorf("name %q: cannot parse timestamp %q: %w", name, tsStr, err)
	}

	return ParsedName{
		Prefix:    prefix,
		Tier:      tier,
		Timestamp: t,
	}, nil
}
