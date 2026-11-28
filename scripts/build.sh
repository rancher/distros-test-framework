#!/bin/bash

set -x
set -eu

DEBUG="${DEBUG:-false}"

TRIM_JOB_NAME=$(basename "$JOB_NAME")

# DOCKERFILE is a job parameter; only ENTRYPOINT-less Jenkins images fit the Jenkinsfile's runtime mounts.
DOCKERFILE="${DOCKERFILE:-scripts/Dockerfile.jenkins}"
case "$DOCKERFILE" in
    scripts/Dockerfile.jenkins | scripts/Dockerfile.rancherday1) ;;
    *)
        echo "ERROR: DOCKERFILE must be scripts/Dockerfile.jenkins or scripts/Dockerfile.rancherday1, got '$DOCKERFILE'" >&2
        exit 1
        ;;
esac

if [ "false" != "${DEBUG}" ]; then
    env | sort | sed -E 's/(.*SECRET.*|.*TOKEN.*|.*PASSWORD.*|.*KEY_ID.*|.*ACCESS_KEY.*|.*PEM.*)=.*/\1=<REDACTED>/'
fi

count=0
while [[ 3 -gt $count ]]; do
    docker build . -f "$DOCKERFILE" -t acceptance-tests-"${TRIM_JOB_NAME}""${BUILD_NUMBER}"

    BUILD_EXIT_CODE=$?
    if [[ $BUILD_EXIT_CODE -eq 0 ]]; then break; fi
    count=$((count + 1))
    echo "Repeating failed Docker build ${count} of 3..."
done

