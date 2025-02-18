#!/usr/bin/env bash

PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

create_binary() {
  go build -o processreport ./cmd/qase/main.go
}

run_qase() {
  ./processreport -f "$latest_log" -p "$PRODUCT"
}

PRODUCT=
latest_log=$(find  ./report -type f -name "rke2_*.log" -o -name "k3s_*.log" | sort -r | head -n 1)

if [[ ! "$latest_log" =~ ^./report/(rke2|k3s)_.*\.log$ ]]; then
  echo "Invalid log file name: $latest_log"
  exit 1
fi

if [[ "$latest_log" =~ ^./report/rke2_.*\.log$ ]]; then
  PRODUCT="rke2"
else
  PRODUCT="k3s"
fi

if [ -f "$latest_log" ]; then
  echo "Uploading $latest_log to Qase"
  create_binary
  run_qase
else
  echo "No log file found for $PRODUCT"
  exit 1
fi

cd ../report
rm -f processreport




