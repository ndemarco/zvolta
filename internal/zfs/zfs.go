package zfs

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Commander executes ZFS commands. The default implementation shells out to the
// zfs CLI. Tests can substitute a mock.
type Commander interface {
	Run(args ...string) (string, error)
}

// CLI implements Commander by calling the zfs binary.
type CLI struct {
	Binary string // path to zfs binary, defaults to "zfs"
}

func (c *CLI) binary() string {
	if c.Binary != "" {
		return c.Binary
	}
	return "zfs"
}

func (c *CLI) Run(args ...string) (string, error) {
	cmd := exec.Command(c.binary(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("zfs %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// Dataset represents a ZFS dataset.
type Dataset struct {
	Name string
}

// Snapshot represents a ZFS snapshot.
type Snapshot struct {
	Name     string // full name: dataset@snapshot
	Dataset  string
	SnapName string // just the part after @
	Creation time.Time
}

// Client provides high-level ZFS operations.
type Client struct {
	Cmd Commander
}

// NewClient creates a Client with the default CLI commander.
func NewClient(binary string) *Client {
	return &Client{Cmd: &CLI{Binary: binary}}
}

// ListDatasets returns all ZFS datasets (filesystems and volumes).
func (c *Client) ListDatasets() ([]Dataset, error) {
	out, err := c.Cmd.Run("list", "-H", "-o", "name", "-t", "filesystem,volume")
	if err != nil {
		return nil, err
	}
	var datasets []Dataset
	for _, line := range splitLines(out) {
		if line != "" {
			datasets = append(datasets, Dataset{Name: line})
		}
	}
	return datasets, nil
}

// GetProperty reads a single ZFS property on a dataset. Returns the value, or
// "-" if the property is not set (ZFS default for user properties).
func (c *Client) GetProperty(dataset, property string) (string, error) {
	out, err := c.Cmd.Run("get", "-H", "-o", "value", property, dataset)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GetProperties reads multiple ZFS properties on a dataset. Returns a map of
// property name to value.
func (c *Client) GetProperties(dataset string, properties []string) (map[string]string, error) {
	propList := strings.Join(properties, ",")
	out, err := c.Cmd.Run("get", "-H", "-o", "property,value", propList, dataset)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(properties))
	for _, line := range splitLines(out) {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result, nil
}

// GetPropertiesRecursive reads a property across a dataset and all its children.
// Returns a map of dataset name to property value.
func (c *Client) GetPropertiesRecursive(dataset, property string) (map[string]string, error) {
	out, err := c.Cmd.Run("get", "-H", "-r", "-o", "name,value", property, dataset)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, line := range splitLines(out) {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result, nil
}

// SetProperty sets a ZFS property on a dataset.
func (c *Client) SetProperty(dataset, property, value string) error {
	_, err := c.Cmd.Run("set", fmt.Sprintf("%s=%s", property, value), dataset)
	return err
}

// CreateSnapshot creates a ZFS snapshot.
func (c *Client) CreateSnapshot(dataset, snapName string) error {
	full := fmt.Sprintf("%s@%s", dataset, snapName)
	_, err := c.Cmd.Run("snapshot", full)
	return err
}

// DestroySnapshot destroys a ZFS snapshot.
func (c *Client) DestroySnapshot(dataset, snapName string) error {
	full := fmt.Sprintf("%s@%s", dataset, snapName)
	_, err := c.Cmd.Run("destroy", full)
	return err
}

// ListSnapshots lists snapshots for a dataset, optionally filtered by a name prefix.
// Results are sorted by creation time (ZFS default).
func (c *Client) ListSnapshots(dataset string) ([]Snapshot, error) {
	out, err := c.Cmd.Run("list", "-H", "-o", "name,creation", "-t", "snapshot",
		"-s", "creation", "-r", "-d", "1", dataset)
	if err != nil {
		return nil, err
	}
	var snaps []Snapshot
	for _, line := range splitLines(out) {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 1 || parts[0] == "" {
			continue
		}
		fullName := parts[0]
		atIdx := strings.Index(fullName, "@")
		if atIdx < 0 {
			continue
		}
		snap := Snapshot{
			Name:     fullName,
			Dataset:  fullName[:atIdx],
			SnapName: fullName[atIdx+1:],
		}
		if len(parts) == 2 {
			// ZFS creation property is a Unix timestamp when using -p, but we
			// use the human-readable default for now. Parse with reference formats.
			snap.Creation, _ = parseZFSTime(strings.TrimSpace(parts[1]))
		}
		snaps = append(snaps, snap)
	}
	return snaps, nil
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// parseZFSTime tries to parse ZFS creation timestamps. ZFS outputs locale-dependent
// date strings. Using -p flag gives Unix epoch which is more reliable.
func parseZFSTime(s string) (time.Time, error) {
	// Common ZFS formats
	formats := []string{
		"Mon Jan  2 15:04 2006",
		"Mon Jan 2 15:04 2006",
		time.UnixDate,
		time.ANSIC,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse ZFS time: %q", s)
}
