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

    QASE_MILESTONE="${CURRENT_MONTH} ${CURRENT_YEAR} Patch Validation"
    echo "QASE_MILESTONE=$QASE_MILESTONE"

    # Get the list of rcs to process from GH action parameter.
    IFS=',' read -r -a rcs_to_process <<<"${RCS}"

    All_RCS=${rcs_to_process[*]}

    # Optional id from the release bot, appended to each run description so the bot can find
    # the runs this dispatch created. Restricted charset: it is spliced into the JSON body.
    if [[ -n "$REQUEST_ID" && ! "$REQUEST_ID" =~ ^[A-Za-z0-9._-]{1,64}$ ]]; then
        echo "Error: invalid REQUEST_ID '$REQUEST_ID'."
        exit 1
    fi
}

# Function to find the milestone with the exact given name. Sets MILESTONE_ID, empty if none exists.
find_milestone() {
    local search
    search=$(jq -rn --arg t "$QASE_MILESTONE" '$t|@uri')

    # A failed search must stop the script: treating it as "no milestone" would create a duplicate.
    local http_code
    RESPONSE=$(curl -sS --request GET -w '\n%{http_code}' \
        --url "https://api.qase.io/v1/milestone/$QASE_PROJECT_CODE?search=$search&limit=100" \
        --header "Token: $QASE_API_TOKEN" --header 'accept: application/json')
    http_code="${RESPONSE##*$'\n'}"
    RESPONSE="${RESPONSE%$'\n'*}"
    if [[ "$http_code" != "200" ]] || ! echo "$RESPONSE" | jq -e '.status == true and (.result.entities | type == "array")' >/dev/null 2>&1; then
        echo "Error: milestone search failed (HTTP $http_code): $RESPONSE"
        exit 1
    fi

    # lowest id wins, so reruns in the same month keep using the first milestone.
    MILESTONE_ID=$(echo "$RESPONSE" | jq --arg t "$QASE_MILESTONE" \
        '[.result.entities[] | select(.title == $t) | .id] | min // empty')
}

# Function to reuse the milestone with given name, or create it. It will return milestone ID.
create_milestone() {
    if [[ -z "$QASE_MILESTONE" ]]; then
        echo "Error: Missing required QASE_MILESTONE."
        exit 1
    fi

    find_milestone
    if [[ -n "$MILESTONE_ID" ]]; then
        echo "Reusing existing milestone ID: $MILESTONE_ID"
        return
    fi

    RESPONSE=$(curl -s --request POST \
        --url "https://api.qase.io/v1/milestone/$QASE_PROJECT_CODE" \
        --header "Token: $QASE_API_TOKEN" --header 'Content-Type: application/json' \
        --data '{
                "title": "'"$QASE_MILESTONE"'"
            }' )

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
                QASE_TEST_PLAN_ID='20'
                IDENTIFIER='k3s1'
            fi

            product=$(echo "$product" | tr '[:lower:]' '[:upper:]')
            TITLE="$product ${CURRENT_MONTH} ${CURRENT_YEAR} Patch Validation for $VERSION+$IDENTIFIER"
            DESCRIPTION="Version: $RC"
            if [[ -n "$REQUEST_ID" ]]; then
                DESCRIPTION="$DESCRIPTION | Release bot request: $REQUEST_ID"
            fi

            create_test_run
        done
    done
}

# Function to create test run with given parameters being: title, description, milestone id, plan id and tag.
create_test_run() {
    TAG_JSON='["'"$QASE_TAG"'"]'

    RESPONSE=$(curl -s --request POST \
        --url "https://api.qase.io/v1/run/$QASE_PROJECT_CODE" \
        --header "Token: $QASE_API_TOKEN" --header 'Content-Type: application/json' \
        --data '{
                "title": "'"$TITLE"'",
                "description": "'"$DESCRIPTION"'",
                "milestone_id": '"$MILESTONE_ID"',
                "tags": '"$TAG_JSON"',
                "include_all_cases": false,
                "plan_id": '"$QASE_TEST_PLAN_ID"'
            }' )
    echo "response status is: $(echo "$RESPONSE" | jq -r '.status // .error // "unknown"')"

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
