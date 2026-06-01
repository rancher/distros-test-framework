#!/bin/bash

set -x
set -eu

DEBUG="${DEBUG:-false}"

env | grep -E '^(AWS|RKE2).*\=.+' | sort > .env
chmod 600 .env

if [ "false" != "${DEBUG}" ]; then
    sed -E 's/(.*SECRET.*|.*TOKEN.*|.*PASSWORD.*|.*KEY_ID.*|.*ACCESS_KEY.*|.*PEM.*)=.*/\1=<REDACTED>/' .env
fi
