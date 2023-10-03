#!/bin/bash

CID="CONTAINER ID"
LOGFILE="./logs/docker_stats.log"
CPU_THRESHOLD=50.0
MEM_THRESHOLD=1073741824

# Check if the logger file exists
if [ ! -f "$LOGFILE" ]; then
    touch "$LOGFILE"
fi

DURATION=$(($(date +%s) + 2400))

while [[ $(date +%s) -lt $DURATION ]]; do
    STATS=$(docker stats --no-stream --format "{{.CPUPerc}},{{.MemUsage}}" "$CID")
    echo "$STATS" >> "$LOGFILE"

    CPU=$(echo "$STATS" | cut -d ',' -f 1)
    MEM=$(echo "$STATS" | cut -d ',' -f 2)

    if (( $(echo "$CPU > $CPU_THRESHOLD" | bc -l) )); then
        echo "Please check: CPU usage is above threshold"
    fi
    if (( $(echo "$MEM > $MEM_THRESHOLD" | bc -l) )); then
        echo "Please check: Memory usage is above threshold"
    fi
    sleep 5
done
