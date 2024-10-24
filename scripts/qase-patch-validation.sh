#!/usr/bin/env bash

# Qase Patch Validation Run Creation Script.
 
PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

CREATE_MILESTONE=${1}
MILESTONE=${2}

validate_params() {
  validate_token
    if [[ -z "$QASE_PROJECT_CODE" ]]; then
        echo "Error: Missing required var PROJECT_CODE."
        exit 1
    fi

    if [[ -z "$PLAN_ID" ]]; then
        echo "Error: Missing required var PLAN_ID."
        exit 1
    fi

    if [[ -z "$TAG" ]]; then
        echo "Error: Missing required var TAG_NAME."
        exit 1
    fi
}

validate_token() {
    if [[ -z "$QASE_API_TOKEN" ]]; then
        echo "Error: Missing required var QASE_API_TOKEN."
        exit 1
    fi
}

create_milestone() {
    if [[ -z "$MILESTONE" ]]; then
        echo "Error: Missing required MILESTONE_NAME."
        exit 1
    fi

     RESPONSE=$(curl --request POST \
        --url "https://api.qase.io/v1/milestone/$QASE_PROJECT_CODE" \
        --header "Token: $QASE_API_TOKEN" \
        --header 'Content-Type: application/json' \
        --data '{
                "title": "'"$MILESTONE"'"
            }')

    # extract milestone ID from response.
    MILESTONE_ID=$(echo "$RESPONSE" | jq '.result.id')
    if [[ -z "$MILESTONE_ID" || "$MILESTONE_ID" == "null" ]]; then
        echo "Failed to create milestone."
        exit 1
    fi

    echo "$MILESTONE_ID"
}

# Function to create test run with given parameters being: title, description, milestone id,plan id and tag.
create_test_run() {
     TAG_JSON='["'"$TAG"'"]'

    RESPONSE=$(curl --request POST \
        --url "https://api.qase.io/v1/run/$QASE_PROJECT_CODE" \
        --header "Token: $QASE_API_TOKEN" \
        --header 'Content-Type: application/json' \
        --data '{
                "title": "'"$TITLE"'",
                "description": "'"$DESCRIPTION"'",
                "milestone_id": '"$MILESTONE_ID"',
                "tags": '"$TAG_JSON"',
                "include_all_cases": false,
                "plan_id": '"$PLAN_ID"'
            }')

    echo "response status is: $RESPONSE"

    # extract run ID from response, fail if not found since its crutial step.
    RUN_ID=$(echo "$RESPONSE" | jq '.result.id')
    if [[ -z "$RUN_ID" || "$RUN_ID" == "null" ]]; then
        echo "Failed to create test run."
        exit 1
    fi

    echo "Created test run with ID: $RUN_ID"
}

main() {
    # If we provide milestone == true, create milestone and return.
    if [[ "$CREATE_MILESTONE" == "true" ]]; then
        validate_token
        create_milestone
        exit 0
    else
      validate_params
      create_test_run
    fi
}

main "$@"