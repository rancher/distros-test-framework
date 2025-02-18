#!/usr/bin/env bash

PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

create_binary() {
if ! command -v go &> /dev/null; then
  echo "Error: golang is not installed"
  exit 1
fi

  echo "Building process report binary..."
  go build -o ./processreport ./cmd/qase/main.go
}

run_qase() {
  echo "Processing $latest_log for $PRODUCT..."
  ./processreport -f "$latest_log" -p "$PRODUCT"
}

# Init variables.
PRODUCT=
latest_log=$(find  ./report -type f -name "rke2_*.log" -o -name "k3s_*.log" | sort -r | head -n 1)

if [[ ! "$latest_log" =~ ^./report/(rke2|k3s)_.*\.log$ ]]; then
  echo "Error: Invalid log file name: $latest_log"
  exit 1
fi

if [[ "$latest_log" =~ ^./report/rke2_.*\.log$ ]]; then
  PRODUCT="rke2"
else
  PRODUCT="k3s"
fi

if [ -f "$latest_log" ]; then
  echo "Found log file: $latest_log"
  create_binary
  run_qase
  echo "Upload complete"
  rm -f processreport
else
  echo "Error: No log file found for $PRODUCT"
  exit 1
fi
