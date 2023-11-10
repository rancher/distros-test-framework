#!/bin/bash

if [ -z "${TEST_DIR}" ]; then
    printf "\n\nTEST DIR: %s is not set\n\n" "${TEST_DIR}"
    exit 1
fi

if [ -z "${IMG_NAME}" ]; then
   printf "\n\nIMG NAME: %s is not set\n\n" "${IMG_NAME}"
   exit 1
fi

case "$TEST_DIR" in
     upgradecluster|versionbump|mixedoscluster|dualstack|validatecluster|createcluster|certrotate)
      printf "\n\nRunning tests for %s\n\n" "${TEST_DIR} on ${ENV_PRODUCT}"
        ;;
    *)
        printf "\n%s is not a go test package\n\n" "${TEST_DIR}"
        exit 1
        ;;
esac

if [ -n "${TEST_DIR}" ]; then
    if [ "${TEST_DIR}" = "upgradecluster" ]; then
        if [ "${TEST_TAG}" = "upgrademanual" ]; then
            go test -timeout=45m -v -tags=upgrademanual -count=1 ./entrypoint/upgradecluster/... -installVersionOrCommit "${INSTALL_VERSION_OR_COMMIT}" -channel "${CHANNEL}"
        elif [ "${TEST_TAG}" = "upgradesuc" ]; then
            go test -timeout=45m -v -tags=upgradesuc -count=1 ./entrypoint/upgradecluster/... -sucUpgradeVersion "${SUC_UPGRADE_VERSION}"
        fi
    elif [ "${TEST_DIR}" = "versionbump" ]; then
        go test -timeout=45m -v -tags=versionbump -count=1 ./entrypoint/versionbump/... \
            -cmd "${CMD}" \
            -expectedValue "${EXPECTED_VALUE}" \
            -expectedValueUpgrade "${VALUE_UPGRADED}" \
            -installVersionOrCommit "${INSTALL_VERSION_OR_COMMIT}" \
            -channel "${CHANNEL}" \
            -testCase "${TEST_CASE}" \
            -deployWorkload "${DEPLOY_WORKLOAD}" \
            -workloadName "${WORKLOAD_NAME}" \
            -description "${DESCRIPTION}"
    elif [ "${TEST_DIR}" = "mixedoscluster" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/mixedoscluster/... -sonobuoyVersion "${SONOBUOYVERSION}"
    elif [ "${TEST_DIR}" = "dualstack" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/dualstack/...
    elif [  "${TEST_DIR}" = "createcluster" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/createcluster/...
    elif [ "${TEST_DIR}" = "validatecluster" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/validatecluster/...
    elif [ "${TEST_DIR}" = "certrotate" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/certrotate/...        
    fi
fi

tail -f /dev/null

