#!/usr/bin/env bash

# Wait for an HTTP(S) endpoint or a TCP host:port target to become reachable.
set -euo pipefail

wait_for="${WAIT_FOR:-}"
retry_seconds="${WAIT_FOR_RETRY_SECONDS:-1}"

if [[ -z "$wait_for" ]]; then
    echo "WAIT_FOR must be an HTTP(S) URL or host:port" >&2
    exit 2
fi

if [[ "$wait_for" =~ ^https?:// ]]; then
    until curl --fail --silent "$wait_for" >/dev/null; do
        sleep "$retry_seconds"
    done
    exit 0
fi

if [[ "$wait_for" != *:* ]]; then
    echo "WAIT_FOR port target must use host:port" >&2
    exit 2
fi

wait_host="${wait_for%:*}"
wait_port="${wait_for##*:}"
if [[ -z "$wait_host" || -z "$wait_port" ]]; then
    echo "WAIT_FOR port target must use host:port" >&2
    exit 2
fi

until (</dev/tcp/"$wait_host"/"$wait_port") >/dev/null 2>&1; do
    sleep "$retry_seconds"
done
