#!/usr/bin/env bash

# Qase Create Test Run Script

# Required parameters:
# --token|          -t <token>                   - Qase API token
# --title|          -n <title>                   - Title of the test run
# --description|    -d <description>             - Description of the test run
# --milestone|      -m <milestone>               - Milestone of the test run

PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

# Default values
QASE_PROJECT_CODE="K3SRKE2"
PLAN_ID=13
TAG="team-rke2"

# Function to validate required parameters
validate_parameters() {
    if [[ -z "$QASE_API_TOKEN" || -z "$RUN_TITLE" || -z "$RUN_DESCRIPTION" || -z "$MILESTONE_TITLE" ]]; then
        echo "Error: Missing required parameters."
        exit 1
    fi
}
# Function to create a milestone
create_milestone() {
    echo "Creating milestone: $MILESTONE_TITLE"
    MILESTONE_RESPONSE=$(curl --request POST \
        --url "https://api.qase.io/v1/milestone/$QASE_PROJECT_CODE" \
        --header "Token: $QASE_API_TOKEN" \
        --header 'Content-Type: application/json' \
        --data '{
            "title": "'"$MILESTONE_TITLE"'"
        }' \
        --silent)

    echo "Milestone response: $MILESTONE_RESPONSE"

    MILESTONE_ID=$(echo "$MILESTONE_RESPONSE" | jq '.result.id')
    if [[ -z "$MILESTONE_ID" || "$MILESTONE_ID" == "null" ]]; then
        echo "Failed to create milestone."
        exit 1
    fi

    return "$MILESTONE_ID"
}

create_test_run() {
    JSON_TAG='["'"$TAG"'"]'

    RUN_RESPONSE=$(curl --request POST \
        --url "https://api.qase.io/v1/run/$QASE_PROJECT_CODE" \
        --header "Token: $QASE_API_TOKEN" \
        --header 'Content-Type: application/json' \
        --data '{
                "title": "'"$RUN_TITLE"'",
                "description": "'"$RUN_DESCRIPTION"'",
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
    parse_parameters "$@"
    validate_parameters
    create_milestone
    create_test_run
}

main "$@"
