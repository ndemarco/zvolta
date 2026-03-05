#!/usr/bin/env bash
set -euo pipefail

# Create config directory if it doesn't exist
mkdir -p /etc/zvolta

# Copy example config if no config exists yet
if [ ! -f /etc/zvolta/zvolta.toml ]; then
    cp /etc/zvolta/zvolta.toml.example /etc/zvolta/zvolta.toml
    echo "Installed default config at /etc/zvolta/zvolta.toml"
fi

# Reload systemd to pick up the unit file
if command -v systemctl &>/dev/null; then
    systemctl daemon-reload
    echo "Run 'systemctl enable --now zvolta' to start the daemon."
fi
