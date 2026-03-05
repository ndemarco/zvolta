package snapshot

import (
	"testing"
	"time"
)

func TestFormatName(t *testing.T) {
	ts := time.Date(2026, 3, 5, 14, 0, 0, 0, time.UTC)
	got := FormatName("zvolta_", TierHourly, ts)
	want := "zvolta_hourly_2026-03-05T14-00-00Z"
	if got != want {
		t.Errorf("FormatName = %q, want %q", got, want)
	}
}

func TestFormatNameConvertsToUTC(t *testing.T) {
	est := time.FixedZone("EST", -5*3600)
	ts := time.Date(2026, 3, 5, 9, 0, 0, 0, est) // 09:00 EST = 14:00 UTC
	got := FormatName("zvolta_", TierHourly, ts)
	want := "zvolta_hourly_2026-03-05T14-00-00Z"
	if got != want {
		t.Errorf("FormatName = %q, want %q", got, want)
	}
}

func TestParseName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		prefix  string
		want    ParsedName
		wantErr bool
	}{
		{
			name:   "valid hourly",
			input:  "zvolta_hourly_2026-03-05T14-00-00Z",
			prefix: "zvolta_",
			want: ParsedName{
				Prefix:    "zvolta_",
				Tier:      TierHourly,
				Timestamp: time.Date(2026, 3, 5, 14, 0, 0, 0, time.UTC),
			},
		},
		{
			name:   "valid frequent",
			input:  "zvolta_frequent_2026-03-05T14-15-00Z",
			prefix: "zvolta_",
			want: ParsedName{
				Prefix:    "zvolta_",
				Tier:      TierFrequent,
				Timestamp: time.Date(2026, 3, 5, 14, 15, 0, 0, time.UTC),
			},
		},
		{
			name:   "custom prefix",
			input:  "mysnaps_daily_2026-01-01T00-00-00Z",
			prefix: "mysnaps_",
			want: ParsedName{
				Prefix:    "mysnaps_",
				Tier:      TierDaily,
				Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name:    "wrong prefix",
			input:   "other_hourly_2026-03-05T14-00-00Z",
			prefix:  "zvolta_",
			wantErr: true,
		},
		{
			name:    "invalid tier",
			input:   "zvolta_biweekly_2026-03-05T14-00-00Z",
			prefix:  "zvolta_",
			wantErr: true,
		},
		{
			name:    "no separator",
			input:   "zvolta_",
			prefix:  "zvolta_",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseName(tt.input, tt.prefix)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseName(%q) expected error, got %+v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseName(%q) unexpected error: %v", tt.input, err)
			}
			if got.Prefix != tt.want.Prefix || got.Tier != tt.want.Tier || !got.Timestamp.Equal(tt.want.Timestamp) {
				t.Errorf("ParseName(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	prefix := "zvolta_"
	ts := time.Date(2026, 6, 15, 8, 30, 0, 0, time.UTC)
	for _, tier := range AllTiers() {
		name := FormatName(prefix, tier, ts)
		parsed, err := ParseName(name, prefix)
		if err != nil {
			t.Errorf("round-trip failed for tier %s: %v", tier, err)
			continue
		}
		if parsed.Tier != tier {
			t.Errorf("tier: got %s, want %s", parsed.Tier, tier)
		}
		if !parsed.Timestamp.Equal(ts) {
			t.Errorf("timestamp: got %v, want %v", parsed.Timestamp, ts)
		}
	}
}

func TestParseTier(t *testing.T) {
	for _, tier := range AllTiers() {
		got, err := ParseTier(string(tier))
		if err != nil {
			t.Errorf("ParseTier(%q) unexpected error: %v", tier, err)
		}
		if got != tier {
			t.Errorf("ParseTier(%q) = %q, want %q", tier, got, tier)
		}
	}

	_, err := ParseTier("invalid")
	if err == nil {
		t.Error("ParseTier(\"invalid\") expected error")
	}
}
