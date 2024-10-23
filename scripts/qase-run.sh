#!/usr/bin/env bash

# Qase Create Test Run Script

# Required parameters:
# QASE_API_TOKEN     Qase API token
# TITLE              Title of the test run
# DESCRIPTION        Description of the test run
# MILESTONE_ID       Milestone id

PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

# Default values
QASE_PROJECT_CODE="K3SRKE2"
PLAN_ID=2
TAG="team-rke2"

# Variables parameters
QASE_API_TOKEN=${QASE_API_TOKEN}
TITLE=${TITLE}
DESCRIPTION=${DESCRIPTION}
MILESTONE_ID=${MILESTONE_ID}

# Function to validate required parameters
validate_parameters() {
    if [[ -z "$QASE_API_TOKEN" || -z "$TITLE" || -z "$DESCRIPTION" || -z "$MILESTONE_ID" ]]; then
        echo "Error: Missing required parameters."
        exit 1
    fi
}

# Function to create test run.
create_test_run() {
    JSON_TAG='["'"$TAG"'"]'

    RUN_RESPONSE=$(curl --request POST \
        --url "https://api.qase.io/v1/run/$QASE_PROJECT_CODE" \
        --header "Token: $QASE_API_TOKEN" \
        --header 'Content-Type: application/json' \
        --data '{
                "title": "'"$TITLE"'",
                "description": "'"$DESCRIPTION"'",
                "milestone_id": '"$MILESTONE_ID"',
                "tags": '"$JSON_TAG"',
                "include_all_cases": false,
                "plan_id": '"$PLAN_ID"'
            }' \
        --silent)

    echo "response status is: $RUN_RESPONSE"

    RUN_ID=$(echo "$RUN_RESPONSE" | jq '.result.id')
    if [[ -z "$RUN_ID" || "$RUN_ID" == "null" ]]; then
        echo "Failed to create test run."
        exit 1
    fi

    echo "Created test run with ID: $RUN_ID"
}

main() {
    validate_parameters
    create_milestone
    create_test_run
}

main
