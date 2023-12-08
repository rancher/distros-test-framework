#!/bin/bash
# script to monitor docker stats
# Usage: ./docker_stats.sh <$IMGNAME>

IMGNAME=$1
printf  "\nMonitoring docker stats for %s\n\n" "$IMGNAME"
echo "CPU Threshold: 50.0%"
CID=$(docker ps -a -q --filter name=acceptance-test-"${IMGNAME}" | head -n 1)
LOGFILE="/tmp/docker_stats-$IMGNAME.log"
CPU_THRESHOLD=50.0
MEM_THRESHOLD=1073741824

if [ ! -f "$LOGFILE" ]; then
    touch "$LOGFILE"
fi

DURATION=$(($(date +%s) + 2400))

while [[ $(date +%s) -lt $DURATION ]]; do
    STATS=$(docker stats --no-stream --format "{{.CPUPerc}},{{.MemUsage}}" "$CID")

    CPU=$(echo "$STATS" | cut -d ',' -f 1 | tr -d '%')
    MEM=$(echo "$STATS" | cut -d ',' -f 2 | cut -d '/' -f 1 | tr -d 'GiB' | tr -d 'MiB' | tr -d 'KiB' | tr -d 'B')

    if (( $(echo "$CPU > $CPU_THRESHOLD" | bc -l) )); then
        echo "$STATS" >> "$LOGFILE"
    fi
    if (( $(echo "$MEM > $MEM_THRESHOLD" | bc -l) )); then
       echo "$STATS" >> "$LOGFILE"
    fi
    sleep 5
done
