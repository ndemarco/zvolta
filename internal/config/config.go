package config

import (
	"fmt"
	"os"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// Config holds the server-wide daemon configuration loaded from TOML.
type Config struct {
	// SnapshotPrefix is the ownership prefix for snapshot names. Default: "zvolta_".
	SnapshotPrefix string `toml:"snapshot_prefix"`

	// ZFSBinary is the path to the zfs binary. Default: "zfs".
	ZFSBinary string `toml:"zfs_binary"`

	// LogLevel controls log verbosity: "debug", "info", "warn", "error". Default: "info".
	LogLevel string `toml:"log_level"`

	// ScheduleOffset shifts all clock-aligned snapshot times by this duration.
	// Positive values delay, negative values advance. Default: 0.
	ScheduleOffset Duration `toml:"schedule_offset"`

	// Datasets lists the root datasets Zvolta should manage. Zvolta reads policies
	// recursively under each root.
	Datasets []string `toml:"datasets"`
}

// Duration wraps time.Duration for TOML string parsing (e.g., "5m", "-30s").
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

// Defaults returns a Config with sane default values.
func Defaults() Config {
	return Config{
		SnapshotPrefix: "zvolta_",
		ZFSBinary:      "zfs",
		LogLevel:       "info",
	}
}

// Load reads a TOML config file and merges it with defaults.
func Load(path string) (Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return cfg, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log_level must be debug, info, warn, or error; got %q", c.LogLevel)
	}
	if c.SnapshotPrefix == "" {
		return fmt.Errorf("snapshot_prefix cannot be empty")
	}
	return nil
}
