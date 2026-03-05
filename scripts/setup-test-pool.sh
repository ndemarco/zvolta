#!/usr/bin/env bash
# Creates a file-backed ZFS pool for integration testing.
# Usage: sudo ./scripts/setup-test-pool.sh [create|destroy]
set -euo pipefail

POOL_NAME="zvolta-test"
POOL_FILE="/tmp/zvolta-test.img"
POOL_SIZE="64M"
DATASET="${POOL_NAME}/data"
MOUNT_BASE="/tmp/zvolta-testmount"

create_pool() {
    echo "Creating test pool: ${POOL_NAME}"

    # Create a sparse file for the pool backing store
    truncate -s "${POOL_SIZE}" "${POOL_FILE}"

    # Create the pool
    zpool create -f -m "${MOUNT_BASE}" "${POOL_NAME}" "${POOL_FILE}"

    # Create a dataset
    zfs create "${DATASET}"

    # Populate with test files (10-20 small files, 1-10KB each)
    local data_dir="${MOUNT_BASE}/data"
    for i in $(seq 1 15); do
        size=$(( (RANDOM % 10 + 1) * 1024 ))
        dd if=/dev/urandom of="${data_dir}/file_${i}.dat" bs=1 count="${size}" 2>/dev/null
    done

    # Add a few text files for variety
    echo "Test document contents for snapshot testing" > "${data_dir}/readme.txt"
    seq 1 100 > "${data_dir}/numbers.txt"
    date > "${data_dir}/timestamp.txt"
    printf 'key=value\nfoo=bar\nsetting=enabled\n' > "${data_dir}/config.ini"
    head -c 2048 /dev/zero > "${data_dir}/zeroed.bin"

    echo "Pool created: ${POOL_NAME}"
    echo "Dataset: ${DATASET}"
    echo "Mounted at: ${data_dir}"
    echo "Files:"
    ls -lh "${data_dir}"
}

destroy_pool() {
    echo "Destroying test pool: ${POOL_NAME}"

    if zpool list "${POOL_NAME}" &>/dev/null; then
        zpool destroy -f "${POOL_NAME}"
    fi

    rm -f "${POOL_FILE}"
    rm -rf "${MOUNT_BASE}"

    echo "Pool destroyed."
}

case "${1:-}" in
    create)
        create_pool
        ;;
    destroy)
        destroy_pool
        ;;
    *)
        echo "Usage: $0 [create|destroy]"
        exit 1
        ;;
esac
