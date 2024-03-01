#!/bin/bash

function validateTestAndImage() {
 if [ -z "${TEST_DIR}" ]; then
     printf "\n\nTEST DIR: %s is not set\n\n" "${TEST_DIR}"
     exit 1
 fi

 if [ -z "${IMG_NAME}" ]; then
    printf "\n\nIMG NAME: %s is not set\n\n" "${IMG_NAME}"
    exit 1
 fi
}

function validateDirName(){
  case "$TEST_DIR" in
       upgradecluster|versionbump|mixedoscluster|dualstack|validatecluster|createcluster|selinux|certrotate|secretsencrypt|restartservice)
       if [[ "$TEST_TAG" != "" ]];
        then
          printf "\n\nRunning tests for %s with %s\n\n" "${TEST_DIR}" "${TEST_TAG}"
        else
          printf "\n\nRunning tests for %s\n\n" "${TEST_DIR} on ${ENV_PRODUCT}"
        fi
          ;;
      *)
          printf "\n\n%s is not a go test package\n\n" "${TEST_DIR}"
          exit 1
          ;;
  esac
}

function run() {
if [ -n "${TEST_DIR}" ]; then
    if [ "${TEST_DIR}" = "upgradecluster" ]; then
        if [ "${TEST_TAG}" = "upgrademanual" ]; then
            go test -timeout=65m -v -tags=upgrademanual -count=1 ./entrypoint/upgradecluster/... -installVersionOrCommit "${INSTALL_VERSION_OR_COMMIT}" -channel "${CHANNEL}"
        elif [ "${TEST_TAG}" = "upgradesuc" ]; then
            go test -timeout=65m -v -tags=upgradesuc -count=1 ./entrypoint/upgradecluster/... -sucUpgradeVersion "${SUC_UPGRADE_VERSION}" -channel "${CHANNEL}"
        fi
    elif [ "${TEST_DIR}" = "versionbump" ]; then
       declare -a OPTS
          OPTS=(-timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=versionbump)
            OPTS+=(-cmd "${CMD}" -expectedValue "${EXPECTED_VALUE}")
             [ -n "${VALUE_UPGRADED}" ] && OPTS+=(-expectedValueUpgrade "${VALUE_UPGRADED}")
             [ -n "${INSTALL_VERSION_OR_COMMIT}" ] && OPTS+=(-installVersionOrCommit "${INSTALL_VERSION_OR_COMMIT}")
             [ -n "${CHANNEL}" ] && OPTS+=(-channel "${CHANNEL}")
             [ -n "${TEST_CASE}" ] && OPTS+=(-testCase "${TEST_CASE}")
             [ -n "${WORKLOAD_NAME}" ] && OPTS+=(-workloadName "${WORKLOAD_NAME}")
             [ -n "${APPLY_WORKLOAD}" ] && OPTS+=(-applyWorkload "${APPLY_WORKLOAD}")
             [ -n "${DELETE_WORKLOAD}" ] && OPTS+=(-deleteWorkload "${DELETE_WORKLOAD}")
             [ -n "${DESCRIPTION}" ] && OPTS+=(-description "${DESCRIPTION}")
      go test "${OPTS[@]}"
    elif [ "${TEST_DIR}" = "mixedoscluster" ]; then
         if [ -n "${SONOBUOYVERSION}" ]; then
                go test -timeout=55m -v -count=1 ./entrypoint/mixedoscluster/... -sonobuoyVersion "${SONOBUOYVERSION}"
            else
                go test -timeout=55m -v -count=1 ./entrypoint/mixedoscluster/...
         fi
    elif [ "${TEST_DIR}" = "dualstack" ]; then
        go test -timeout=55m -v -count=1 ./entrypoint/dualstack/...
    elif [  "${TEST_DIR}" = "createcluster" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/createcluster/...
    elif [ "${TEST_DIR}" = "validatecluster" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/validatecluster/...
    elif [ "${TEST_DIR}" = "selinux" ]; then
        go test -timeout=50m -v -count=1 ./entrypoint/selinux/...
    elif [ "${TEST_DIR}" = "certrotate" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/certrotate/...
    elif [ "${TEST_DIR}" = "secretsencrypt" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/secretsencrypt/...        
    elif [ "${TEST_DIR}" = "restartservice" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/restartservice/...
    fi
fi
}

main() {
  validateTestAndImage
  validateDirName
  run
  tail -f /dev/null
}

main "$@"
