package policy

import (
	"testing"

	"github.com/ndemarco/zvolta/internal/snapshot"
	"github.com/ndemarco/zvolta/internal/zfs"
)

type mockCommander struct {
	outputs map[string]string
}

func (m *mockCommander) Run(args ...string) (string, error) {
	key := ""
	for i, a := range args {
		if i > 0 {
			key += " "
		}
		key += a
	}
	if out, ok := m.outputs[key]; ok {
		return out, nil
	}
	return "", nil
}

func propsOutput() string {
	return "org.zvolta:autosnap\ton\n" +
		"org.zvolta:autoprune\ton\n" +
		"org.zvolta:samba-expose\t-\n" +
		"org.zvolta:naming-algorithm\t-\n" +
		"org.zvolta:snapshot-frequent\t12\n" +
		"org.zvolta:snapshot-frequent-period\t5\n" +
		"org.zvolta:snapshot-hourly\t24\n" +
		"org.zvolta:snapshot-daily\t30\n" +
		"org.zvolta:snapshot-weekly\t4\n" +
		"org.zvolta:snapshot-monthly\t12\n" +
		"org.zvolta:snapshot-yearly\t3\n"
}

func TestResolve(t *testing.T) {
	mock := &mockCommander{outputs: make(map[string]string)}
	allProps := AllProperties()
	propList := ""
	for i, p := range allProps {
		if i > 0 {
			propList += ","
		}
		propList += p
	}
	mock.outputs["get -H -o property,value "+propList+" tank/data"] = propsOutput()

	engine := &Engine{ZFS: &zfs.Client{Cmd: mock}}
	p, err := engine.Resolve("tank/data")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !p.AutoSnap {
		t.Error("autosnap should be true")
	}
	if !p.AutoPrune {
		t.Error("autoprune should be true")
	}
	if p.FrequentCount != 12 {
		t.Errorf("frequent = %d, want 12", p.FrequentCount)
	}
	if p.FrequentPeriod != 5 {
		t.Errorf("frequent period = %d, want 5", p.FrequentPeriod)
	}
	if p.HourlyCount != 24 {
		t.Errorf("hourly = %d, want 24", p.HourlyCount)
	}
	if p.DailyCount != 30 {
		t.Errorf("daily = %d, want 30", p.DailyCount)
	}
	if p.WeeklyCount != 4 {
		t.Errorf("weekly = %d, want 4", p.WeeklyCount)
	}
	if p.MonthlyCount != 12 {
		t.Errorf("monthly = %d, want 12", p.MonthlyCount)
	}
	if p.YearlyCount != 3 {
		t.Errorf("yearly = %d, want 3", p.YearlyCount)
	}
}

func TestRetentionFor(t *testing.T) {
	p := &Policy{
		FrequentCount: 12,
		HourlyCount:   24,
		DailyCount:    30,
		WeeklyCount:   4,
		MonthlyCount:  12,
		YearlyCount:   3,
	}

	tests := []struct {
		tier snapshot.Tier
		want int
	}{
		{snapshot.TierFrequent, 12},
		{snapshot.TierHourly, 24},
		{snapshot.TierDaily, 30},
		{snapshot.TierWeekly, 4},
		{snapshot.TierMonthly, 12},
		{snapshot.TierYearly, 3},
	}

	for _, tt := range tests {
		got := p.RetentionFor(tt.tier)
		if got != tt.want {
			t.Errorf("RetentionFor(%s) = %d, want %d", tt.tier, got, tt.want)
		}
	}
}

func TestIsEnabled(t *testing.T) {
	p := &Policy{
		AutoSnap:       true,
		FrequentCount:  12,
		FrequentPeriod: 5,
		HourlyCount:    24,
		DailyCount:     0, // disabled
	}

	if !p.IsEnabled(snapshot.TierFrequent) {
		t.Error("frequent should be enabled")
	}
	if !p.IsEnabled(snapshot.TierHourly) {
		t.Error("hourly should be enabled")
	}
	if p.IsEnabled(snapshot.TierDaily) {
		t.Error("daily should not be enabled (count=0)")
	}

	// AutoSnap off disables everything
	p.AutoSnap = false
	if p.IsEnabled(snapshot.TierHourly) {
		t.Error("hourly should not be enabled when autosnap=off")
	}
}

func TestFrequentRequiresMinPeriod(t *testing.T) {
	p := &Policy{
		AutoSnap:       true,
		FrequentCount:  12,
		FrequentPeriod: 3, // too low
	}

	if p.IsEnabled(snapshot.TierFrequent) {
		t.Error("frequent should not be enabled with period < 5")
	}
}

func TestValidateRejectsLowPeriod(t *testing.T) {
	p := &Policy{
		FrequentCount:  12,
		FrequentPeriod: 3,
	}
	if err := p.validate(); err == nil {
		t.Error("expected validation error for low frequent period")
	}
}

func TestParseBool(t *testing.T) {
	for _, s := range []string{"on", "ON", "yes", "YES", "true", "TRUE"} {
		if !parseBool(s) {
			t.Errorf("parseBool(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"off", "no", "false", "-", ""} {
		if parseBool(s) {
			t.Errorf("parseBool(%q) = true, want false", s)
		}
	}
}

func TestParseInt(t *testing.T) {
	if parseInt("24") != 24 {
		t.Error("parseInt(24) failed")
	}
	if parseInt("-") != 0 {
		t.Error("parseInt(-) should be 0")
	}
	if parseInt("") != 0 {
		t.Error("parseInt('') should be 0")
	}
	if parseInt("abc") != 0 {
		t.Error("parseInt(abc) should be 0")
	}
}
