package policy

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ndemarco/zvolta/internal/snapshot"
	"github.com/ndemarco/zvolta/internal/zfs"
)

const propertyPrefix = "org.zvolta:"

// Property names (without the org.zvolta: prefix for internal use).
const (
	PropAutoSnap        = "autosnap"
	PropAutoPrune       = "autoprune"
	PropSambaExpose     = "samba-expose"
	PropNamingAlgorithm = "naming-algorithm"
	PropFrequent        = "snapshot-frequent"
	PropFrequentPeriod  = "snapshot-frequent-period"
	PropHourly          = "snapshot-hourly"
	PropDaily           = "snapshot-daily"
	PropWeekly          = "snapshot-weekly"
	PropMonthly         = "snapshot-monthly"
	PropYearly          = "snapshot-yearly"
)

// AllProperties returns all org.zvolta: property names (fully qualified).
func AllProperties() []string {
	short := []string{
		PropAutoSnap, PropAutoPrune, PropSambaExpose, PropNamingAlgorithm,
		PropFrequent, PropFrequentPeriod,
		PropHourly, PropDaily, PropWeekly, PropMonthly, PropYearly,
	}
	full := make([]string, len(short))
	for i, s := range short {
		full[i] = propertyPrefix + s
	}
	return full
}

// Policy holds the resolved snapshot policy for a single dataset.
type Policy struct {
	Dataset        string
	AutoSnap       bool
	AutoPrune      bool
	FrequentCount  int
	FrequentPeriod int // minutes
	HourlyCount    int
	DailyCount     int
	WeeklyCount    int
	MonthlyCount   int
	YearlyCount    int
}

// RetentionFor returns the retention count for a given tier.
func (p *Policy) RetentionFor(tier snapshot.Tier) int {
	switch tier {
	case snapshot.TierFrequent:
		return p.FrequentCount
	case snapshot.TierHourly:
		return p.HourlyCount
	case snapshot.TierDaily:
		return p.DailyCount
	case snapshot.TierWeekly:
		return p.WeeklyCount
	case snapshot.TierMonthly:
		return p.MonthlyCount
	case snapshot.TierYearly:
		return p.YearlyCount
	default:
		return 0
	}
}

// IsEnabled returns true if a tier has a non-zero retention count (and for
// frequent, a valid period).
func (p *Policy) IsEnabled(tier snapshot.Tier) bool {
	if !p.AutoSnap {
		return false
	}
	count := p.RetentionFor(tier)
	if count <= 0 {
		return false
	}
	if tier == snapshot.TierFrequent {
		return p.FrequentPeriod >= 5
	}
	return true
}

// Engine resolves policies for datasets by reading ZFS custom properties.
type Engine struct {
	ZFS *zfs.Client
}

// Resolve reads ZFS properties for a dataset and returns its effective policy.
// ZFS handles property inheritance natively — child datasets inherit parent
// properties unless overridden. We just read the effective value.
func (e *Engine) Resolve(dataset string) (Policy, error) {
	props, err := e.ZFS.GetProperties(dataset, AllProperties())
	if err != nil {
		return Policy{}, fmt.Errorf("reading properties for %s: %w", dataset, err)
	}

	p := Policy{Dataset: dataset}

	p.AutoSnap = parseBool(props[propertyPrefix+PropAutoSnap])
	p.AutoPrune = parseBool(props[propertyPrefix+PropAutoPrune])
	p.FrequentCount = parseInt(props[propertyPrefix+PropFrequent])
	p.FrequentPeriod = parseInt(props[propertyPrefix+PropFrequentPeriod])
	p.HourlyCount = parseInt(props[propertyPrefix+PropHourly])
	p.DailyCount = parseInt(props[propertyPrefix+PropDaily])
	p.WeeklyCount = parseInt(props[propertyPrefix+PropWeekly])
	p.MonthlyCount = parseInt(props[propertyPrefix+PropMonthly])
	p.YearlyCount = parseInt(props[propertyPrefix+PropYearly])

	if err := p.validate(); err != nil {
		return p, fmt.Errorf("policy for %s: %w", dataset, err)
	}

	return p, nil
}

// ResolveAll reads policies for a root dataset and all its children that have
// autosnap enabled.
func (e *Engine) ResolveAll(rootDataset string) ([]Policy, error) {
	// Check which datasets under this root have autosnap set
	autosnaps, err := e.ZFS.GetPropertiesRecursive(rootDataset, propertyPrefix+PropAutoSnap)
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", rootDataset, err)
	}

	var policies []Policy
	for ds, val := range autosnaps {
		if !parseBool(val) {
			continue
		}
		p, err := e.Resolve(ds)
		if err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}

	return policies, nil
}

func (p *Policy) validate() error {
	if p.FrequentCount > 0 && p.FrequentPeriod < 5 {
		return fmt.Errorf("snapshot-frequent-period must be >= 5 minutes when frequent snapshots are enabled; got %d", p.FrequentPeriod)
	}
	return nil
}

func parseBool(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "on" || s == "yes" || s == "true"
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "-" || s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// String returns a human-readable summary of the policy.
func (p *Policy) String() string {
	if !p.AutoSnap {
		return fmt.Sprintf("%s: autosnap=off", p.Dataset)
	}
	parts := []string{fmt.Sprintf("%s: autosnap=on autoprune=%v", p.Dataset, p.AutoPrune)}
	if p.FrequentCount > 0 {
		parts = append(parts, fmt.Sprintf("frequent=%d@%dmin", p.FrequentCount, p.FrequentPeriod))
	}
	for _, tier := range []struct {
		name  string
		count int
	}{
		{"hourly", p.HourlyCount},
		{"daily", p.DailyCount},
		{"weekly", p.WeeklyCount},
		{"monthly", p.MonthlyCount},
		{"yearly", p.YearlyCount},
	} {
		if tier.count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", tier.name, tier.count))
		}
	}
	return strings.Join(parts, " ")
}
