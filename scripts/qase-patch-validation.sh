#!/usr/bin/env bash

# Qase Patch Validation Run Creation Script.

PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

validate_token() {
    if [[ -z "$QASE_API_TOKEN" ]]; then
        echo "Error: Missing required var QASE_API_TOKEN."
        exit 1
    fi
}

set_vars() {
    QASE_PROJECT_CODE='K3SRKE2'
    QASE_TAG='team-rke2'
    CURRENT_MONTH="$(date +"%B")"
    CURRENT_YEAR="$(date +"%Y")"

    QASE_MILESTONE="${CURRENT_MONTH}-${CURRENT_YEAR} Patch release"
    echo "QASE_MILESTONE=$QASE_MILESTONE"

    # Get the list of rcs to process from GH action parameter.
    IFS=',' read -r -a rcs_to_process <<<"${RCS}"

    All_RCS=${rcs_to_process[*]}
}

# Function to create milestone with given name. It will return milestone ID.
create_milestone() {
    if [[ -z "$QASE_MILESTONE" ]]; then
        echo "Error: Missing required QASE_MILESTONE."
        exit 1
    fi

    RESPONSE=$(curl --request POST \
        --url "https://api.qase.io/v1/milestone/$QASE_PROJECT_CODE" \
        --header "Token: $QASE_API_TOKEN" \
        --header 'Content-Type: application/json' \
        --data '{
                "title": "'"$QASE_MILESTONE"'"
            }')

    # extract milestone ID from response.
    MILESTONE_ID=$(echo "$RESPONSE" | jq '.result.id')
    if [[ -z "$MILESTONE_ID" || "$MILESTONE_ID" == "null" ]]; then
        echo "Failed to create milestone."
        exit 1
    fi

    echo "$MILESTONE_ID"
}

process() {
    read -r -a rcs <<<"${All_RCS}"
    read -r -a products <<<"rke2 k3s"

    versions=()
    for rc  in "${rcs[@]}"; do
      version="${rc%-rc*}"
      versions+=("$version")
    done

    for product in "${products[@]}"; do
        for ((i = 0; i < ${#versions[@]}; i++)); do

            #we iterate by modulo to be safer.
            VERSION="${versions[$((i % ${#versions[@]}))]}"
            RC="${rcs[$((i % ${#rcs[@]}))]}"

            if [ "$product" == "rke2" ]; then
                QASE_TEST_PLAN_ID='14'
                IDENTIFIER='rke2r1'
            elif [ "$product" == "k3s" ]; then
                QASE_TEST_PLAN_ID='15'
                IDENTIFIER='k3s1'
            fi

            TITLE="Patch Validation $product ${CURRENT_MONTH}-${CURRENT_YEAR} $VERSION+$IDENTIFIER"
            DESCRIPTION="rc Version: $RC"

            create_test_run
        done
    done
}

# Function to create test run with given parameters being: title, description, milestone id, plan id and tag.
create_test_run() {
    TAG_JSON='["'"$QASE_TAG"'"]'

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
                "plan_id": '"$QASE_TEST_PLAN_ID"'
            }')
    echo "response status is: $RESPONSE"

    echo "Created Qase Test Run with:"
    echo "Title: $TITLE"
    echo "Description: $DESCRIPTION"
    echo "Milestone:  $MILESTONE_ID"

    # extract run ID from response, fail if not found since its crutial step.
    RUN_ID=$(echo "$RESPONSE" | jq '.result.id')
    if [[ -z "$RUN_ID" || "$RUN_ID" == "null" ]]; then
        echo "Failed to create test run."
        exit 1
    fi

    echo "Created test run with ID: $RUN_ID"
}

main() {
    validate_token
    set_vars
    create_milestone
    process
}

main "$@"
