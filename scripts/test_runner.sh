#!/bin/bash

function validate_test_image() {
 if [ -z "${TEST_DIR}" ]; then
     printf "\n\nTEST DIR: %s is not set\n\n" "${TEST_DIR}"
     exit 1
 fi

 if [ -z "${IMG_NAME}" ]; then
    printf "\n\nIMG NAME: %s is not set\n\n" "${IMG_NAME}"
    exit 1
 fi
}

function validate_dir(){
  case "$TEST_DIR" in
       upgradecluster|versionbump|mixedoscluster|dualstack|validatecluster|createcluster|selinux|clusterrestore|\
       certrotate|secretsencrypt|restartservice|deployrancher|clusterreset|rebootinstances|airgap|ipv6only|conformance|killalluninstall)
      if [[ "$TEST_DIR" == "upgradecluster" ]];
        then
            case "$TEST_TAG" in
                upgrademanual|upgradesuc|upgradereplacement)
                ;;
                *)
                printf "\n\n%s is not a valid test tag for %s\n\n" "${TEST_TAG}" "${TEST_DIR}"
                exit 1
                ;;
            esac
       fi
       if [[ "$TEST_DIR" == "airgap" ]];
        then
            case "$TEST_TAG" in
                privateregistry|systemdefaultregistry|tarball)
                ;;
                *)
                printf "\n\n%s is not a valid test tag for %s\n\n" "${TEST_TAG}" "${TEST_DIR}"
                exit 1
                ;;
            esac
       fi
       if [[ "$TEST_TAG" != "" ]];
        then
          printf "\n\nRunning ${ENV_PRODUCT} tests for %s with %s\n\n" "${TEST_DIR}" "${TEST_TAG}"
        else
          printf "\n\nRunning ${ENV_PRODUCT} tests for %s\n\n" "${TEST_DIR}"
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
        elif [ "${TEST_TAG}" = "upgradereplacement" ]; then
            go test -timeout=120m -v -tags=upgradereplacement -count=1 ./entrypoint/upgradecluster/... -installVersionOrCommit "${INSTALL_VERSION_OR_COMMIT}" -channel "${CHANNEL}"
    fi
    elif [ "${TEST_DIR}" = "versionbump" ]; then
        declare -a OPTS
          OPTS=(-timeout=65m -v -count=1 ./entrypoint/versionbump/... -tags="${TEST_TAG}")
            OPTS+=(-cmd "${CMD}" -expectedValue "${EXPECTED_VALUE}")
             [ -n "${VALUE_UPGRADED}" ] && OPTS+=(-expectedValueUpgrade "${VALUE_UPGRADED}")
             [ -n "${INSTALL_VERSION_OR_COMMIT}" ] && OPTS+=(-installVersionOrCommit "${INSTALL_VERSION_OR_COMMIT}")
             [ -n "${CHANNEL}" ] && OPTS+=(-channel "${CHANNEL}")
             [ -n "${TEST_CASE}" ] && OPTS+=(-testCase "${TEST_CASE}")
             [ -n "${WORKLOAD_NAME}" ] && OPTS+=(-workloadName "${WORKLOAD_NAME}")
             [ -n "${APPLY_WORKLOAD}" ] && OPTS+=(-applyWorkload "${APPLY_WORKLOAD}")
             [ -n "${DELETE_WORKLOAD}" ] && OPTS+=(-deleteWorkload "${DELETE_WORKLOAD}")
             [ -n "${DESCRIPTION}" ] && OPTS+=(-description "${DESCRIPTION}")
             [ -n "${DEBUG_MODE}" ] && OPTS+=(-debug "${DEBUG_MODE}")
        go test "${OPTS[@]}"
    elif [ "${TEST_DIR}" = "mixedoscluster" ]; then
         if [ -n "${SONOBUOY_VERSION}" ]; then
            go test -timeout=55m -v -count=1 ./entrypoint/mixedoscluster/... -sonobuoyVersion "${SONOBUOY_VERSION}"
        else
            go test -timeout=55m -v -count=1 ./entrypoint/mixedoscluster/...
         fi
    elif [ "${TEST_DIR}" = "deployrancher" ]; then
        declare -a OPTS
          OPTS=(-timeout=45m -v -count=1 ./entrypoint/deployrancher/... -tags=deployrancher)
            [ -n "${CERT_MANAGER_VERSION}" ] && OPTS+=(-certManagerVersion "${CERT_MANAGER_VERSION}")
            [ -n "${CHARTS_VERSION}" ] && OPTS+=(-chartsVersion "${CHARTS_VERSION}")
            [ -n "${CHARTS_REPO_NAME}" ] && OPTS+=(-chartsRepoName "${CHARTS_REPO_NAME}")
            [ -n "${CHARTS_REPO_URL}" ] && OPTS+=(-chartsRepoUrl "${CHARTS_REPO_URL}")
            [ -n "${CHARTS_ARGS}" ] && OPTS+=(-chartsArgs "${CHARTS_ARGS}")
        go test "${OPTS[@]}"
    elif [ "${TEST_DIR}" = "dualstack" ]; then
        go test -timeout=65m -v -count=1 ./entrypoint/dualstack/...
    elif [ "${TEST_DIR}" = "ipv6only" ]; then
        go test -timeout=65m -v -count=1 ./entrypoint/ipv6only/...
    elif [  "${TEST_DIR}" = "createcluster" ]; then
        go test -timeout=60m -v -count=1 ./entrypoint/createcluster/...
    elif [ "${TEST_DIR}" = "validatecluster" ]; then
        go test -timeout=65m -v -count=1 ./entrypoint/validatecluster/... -destroy "${DESTROY}" -killallUninstall "${KILLALLUNINSTALL}"
    elif [ "${TEST_DIR}" = "selinux" ]; then
        go test -timeout=65m -v -count=1 ./entrypoint/selinux/...
    elif [ "${TEST_DIR}" = "certrotate" ]; then
        go test -timeout=65m -v -count=1 ./entrypoint/certrotate/...
    elif [ "${TEST_DIR}" = "secretsencrypt" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/secretsencrypt/...
    elif [ "${TEST_DIR}" = "restartservice" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/restartservice/...
    elif [ "${TEST_DIR}" = "clusterreset" ]; then
        go test -timeout=120m -v -count=1 ./entrypoint/clusterreset/...
    elif [ "${TEST_DIR}" = "rebootinstances" ]; then
        go test -timeout=120m -v -count=1 ./entrypoint/rebootinstances/...
    elif [ "${TEST_DIR}" = "airgap" ]; then
        declare -a OPTS
        if [[ "${TEST_TAG}" == "privateregistry" ]] || [[ "${TEST_TAG}" == "systemdefaultregistry" ]]; then
          OPTS=(-timeout=60m -v -count=1 ./entrypoint/airgap/... -tags="${TEST_TAG}" -destroy "${DESTROY}")
            [ -n "${IMAGE_REGISTRY_URL}" ] && OPTS+=(-imageRegistryUrl "${IMAGE_REGISTRY_URL}")
            [ -n "${REGISTRY_USERNAME}" ] && OPTS+=(-registryUsername "${REGISTRY_USERNAME}")
            [ -n "${REGISTRY_PASSWORD}" ] && OPTS+=(-registryPassword "${REGISTRY_PASSWORD}")
        elif [[ "${TEST_TAG}" == "tarball" ]]; then
          OPTS=(-timeout=60m -v -count=1 ./entrypoint/airgap/... -tags="${TEST_TAG}" -destroy "${DESTROY}")
            [ -n "${TARBALL_TYPE}" ] && OPTS+=(-tarballType "${TARBALL_TYPE}")
        fi
        go test "${OPTS[@]}"
    elif [ "${TEST_DIR}" = "clusterrestore" ]; then
        declare -a OPTS
          OPTS=(-timeout=45m -v -count=1 ./entrypoint/clusterrestore/... )
           [ -n "${S3_BUCKET}" ] && OPTS+=(-s3Bucket "${S3_BUCKET}")
           [ -n "${S3_FOLDER}" ] && OPTS+=(-s3Folder "${S3_FOLDER}")
           [ -n "${CHANNEL}"   ] && OPTS+=(-channel "${CHANNEL}")
        go test "${OPTS[@]}"
    elif [ "${TEST_DIR}" = "conformance" ]; then
        declare -a OPTS
          OPTS=(-timeout=260m -v -count=1 ./entrypoint/conformance/... )
            [ -n "${SONOBUOY_VERSION}" ] && OPTS+=(-sonobuoyVersion "${SONOBUOY_VERSION}")
        go test "${OPTS[@]}" --ginkgo.timeout=260m
    elif [ "${TEST_DIR}" = "killalluninstall" ]; then
        go test -timeout=120m -v -count=1 ./entrypoint/killalluninstall/... -destroy "${DESTROY}"
    fi
fi
}

main() {
  validate_test_image
  validate_dir
  run
  tail -f /dev/null
}

main "$@"
