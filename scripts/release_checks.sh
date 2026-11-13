#!/bin/sh

while getopts v:l:p:dh OPTION
do 
    case "${OPTION}"
        in
        v) 
            INPUT=${OPTARG}
            LTS="false"
        ;;
        l) 
            INPUT=${OPTARG}
            LTS="true"
        ;;
        p) PAGE_SIZE=${OPTARG};;
        d) DEBUG="true"
           echo "DEBUG mode is ON"
           ;;
        h|?)
            echo "
        Usage: 
            
            $(basename "$0") [-v <version_comma_separated> ] [-p <page_size_integer_value> ] [-d]

            -d: debug. If flag is used, debug mode is ON and prints the curl commands executed.
            -v: version to test. Ex: v1.31.6-rc1+rke2r1,v1.31.6-rc1+k3s1,v1.31.5+rke2r1
            -l: Prime-only/LTS RKE2 version to test (index, registry and RPMs). Ex: v1.32.13-rc1+rke2r3
            LTS_RPM_TARGETS: comma-separated RPM targets; default slemicro/x86_64.
            -p: page size for curl commands to fetch data from docker hub or github. Default is 200.
            -h: help - usage is displayed
            "
            exit 1
            ;;
        esac
done

# Set defaults if not provided
if [ -z "${INPUT}" ]; then
    echo "Provide public versions with -v or Prime-only/LTS RKE2 versions with -l."
    exit 1
fi
if [ -z "${DEBUG}" ]; then
    DEBUG="false"
fi
if [ -z "${PAGE_SIZE}" ]; then
    PAGE_SIZE=200
fi

SCRIPT_DIR=$(CDPATH='' cd "$(dirname "$0")" && pwd) || exit 1
WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/release-checks.XXXXXX") || exit 1
trap 'rm -rf "$WORK_DIR"' EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
cd "$WORK_DIR" || exit 1
FAILURE_FILE="failure_results"

debug_log () {
    if [ "${DEBUG}" = true ]; then
        echo "$ $1"
    fi
}

verify_count () {
    # $1 count $2 expected_count $3 log item we are checking
    COUNT="$1"
    EXPECTED_COUNT="$2"
    DESCRIPTION="$3"

    if [ "${COUNT}" -eq "${EXPECTED_COUNT}" ] || [ "${COUNT}" -gt "${EXPECTED_COUNT}" ]; then
        echo "PASS: ${DESCRIPTION} Count is ${COUNT}."
    else
        echo "FAIL: ${VERSION}: Not found enough ${DESCRIPTION}. Expected count ${EXPECTED_COUNT} but got ${COUNT}." | tee -a "${FAILURE_FILE}"
    fi
}

verify_system_agent_installers () {
    SAI_OUTPUT_FILE="sys_agent_installers"
    SYSTEM_AGENT_INSTALLER_URL="https://registry.hub.docker.com/v2/repositories/rancher/system-agent-installer-${PRODUCT}/tags?page_size=${PAGE_SIZE}"
    
    printf '\n==== VERIFY SYSTEM AGENT INSTALLER FOR Product: %s Version Prefix: %s Version Suffix: %s: ====\n' "${PRODUCT}" "${VERSION_PREFIX}" "${VERSION_SUFFIX}"

    if echo "${VERSION_PREFIX}" | grep -q "rc"; then
        debug_log "curl -L -s \"${SYSTEM_AGENT_INSTALLER_URL}\" | jq -r \".results[].name\" | grep ${VERSION_PREFIX} | grep ${VERSION_SUFFIX} | tee -a \"${SAI_OUTPUT_FILE}\""
        curl -L -s "${SYSTEM_AGENT_INSTALLER_URL}" | jq -r ".results[].name" | grep "${VERSION_PREFIX}" | grep "${VERSION_SUFFIX}" | tee -a "${SAI_OUTPUT_FILE}"
    else
        debug_log "curl -L -s \"${SYSTEM_AGENT_INSTALLER_URL}\" | jq -r \".results[].name\" | grep ${VERSION_PREFIX} | grep ${VERSION_SUFFIX} | grep -v \"rc\" | tee -a \"${SAI_OUTPUT_FILE}\""
        curl -L -s "${SYSTEM_AGENT_INSTALLER_URL}" | jq -r ".results[].name" | grep "${VERSION_PREFIX}" | grep "${VERSION_SUFFIX}" | grep -v "rc" | tee -a "${SAI_OUTPUT_FILE}"
    fi

    SAI_COUNT=$(wc -l < "${SAI_OUTPUT_FILE}")
    WINDOWS_COUNT=$(grep -c "windows" "${SAI_OUTPUT_FILE}")
    LINUX_ARM_COUNT=$(grep -c "linux-arm64" "${SAI_OUTPUT_FILE}")
    LINUX_AMD_COUNT=$(grep -c "linux-amd64" "${SAI_OUTPUT_FILE}")
    
    if [ "${PRODUCT}" = "rke2" ]; then
        verify_count "${SAI_COUNT}" "5" "Total images"
        verify_count "${WINDOWS_COUNT}" "2" "Windows images"
    else
        verify_count "${SAI_COUNT}" "3" "Total images"
    fi

    verify_count "${LINUX_ARM_COUNT}" "1" "Linux arm images"
    verify_count "${LINUX_AMD_COUNT}" "1" "linux-amd64 images"

    rm -rf "${SAI_OUTPUT_FILE}"
}

verify_upgrade_images () {
    UPG_OUTPUT_FILE="upgrade_images"
    UPGRADE_IMAGES_URL="https://registry.hub.docker.com/v2/repositories/rancher/${PRODUCT}-upgrade/tags?page_size=${PAGE_SIZE}"
    
    printf '\n==== VERIFY UPGRADE IMAGES FOR Product: %s Version Prefix: %s Version Suffix: %s: ====\n' "${PRODUCT}" "${VERSION_PREFIX}" "${VERSION_SUFFIX}"
    
    if echo "${VERSION_PREFIX}" | grep -q "rc"; then
        debug_log "curl -L -s ${UPGRADE_IMAGES_URL} | jq -r \".results[].name\" | grep ${VERSION_PREFIX} | grep ${VERSION_SUFFIX}"
        curl -L -s "${UPGRADE_IMAGES_URL}" | jq -r ".results[].name" | grep "${VERSION_PREFIX}" | grep "${VERSION_SUFFIX}" | tee -a "${UPG_OUTPUT_FILE}"
    else
        debug_log "curl -L -s ${UPGRADE_IMAGES_URL} | jq -r \".results[].name\" | grep ${VERSION_PREFIX} | grep ${VERSION_SUFFIX} | grep -v \"rc\""
        curl -L -s "${UPGRADE_IMAGES_URL}" | jq -r ".results[].name" | grep "${VERSION_PREFIX}" | grep "${VERSION_SUFFIX}" | grep -v "rc" | tee -a "${UPG_OUTPUT_FILE}"
    fi
    
    UPGRADE_COUNT=$(wc -l < "${UPG_OUTPUT_FILE}")
    verify_count "${UPGRADE_COUNT}" "1" "Upgrade images"
    
    rm -rf "${UPG_OUTPUT_FILE}"
}

verify_releases () {
    RELEASES_FILE="releases"
    printf '\n==== VERIFY RELEASES FOR Product: %s Version Prefix: %s Version Suffix: %s: ====\n' "${PRODUCT}" "${VERSION_PREFIX}" "${VERSION_SUFFIX}"

    if [ "${PRODUCT}" = "rke2" ]; then
        RELEASES_URL="https://api.github.com/repos/rancher/rke2/releases"
    else
        RELEASES_URL="https://api.github.com/repos/k3s-io/k3s/releases"
    fi

    if echo "${VERSION_PREFIX}" | grep -q "rc"; then
        debug_log "curl -s -H \"Accept: application/vnd.github+json\" ${RELEASES_URL} | jq '.[].tag_name' | grep ${VERSION_PREFIX} | grep ${VERSION_SUFFIX}"
        curl -s -H "Accept: application/vnd.github+json" "${RELEASES_URL}" | jq '.[].tag_name' | grep "${VERSION_PREFIX}" | grep "${VERSION_SUFFIX}" | tee -a "${RELEASES_FILE}"
    else
        debug_log "curl -s -H \"Accept: application/vnd.github+json\" ${RELEASES_URL} | jq '.[].tag_name' | grep ${VERSION_PREFIX} | grep ${VERSION_SUFFIX} | grep -v \"rc\""
        curl -s -H "Accept: application/vnd.github+json" "${RELEASES_URL}" | jq '.[].tag_name' | grep "${VERSION_PREFIX}" | grep "${VERSION_SUFFIX}" | grep -v "rc" | tee -a "${RELEASES_FILE}"
    fi

    RELEASES_COUNT=$(wc -l < "${RELEASES_FILE}")
    verify_count "${RELEASES_COUNT}" "1" "Release versions"

    rm -rf "${RELEASES_FILE}"
}

verify_release_asset_count_rke2 () {
    printf '\n==== VERIFY ASSET COUNT FOR RKE2 VERSION: %s: ====\n' "${VERSION}"
    debug_log "curl -sS -H \"Accept: application/vnd.github+json\" \"https://api.github.com/repos/rancher/rke2/releases/tags/${VERSION}\" | jq '.assets | length'"
    
    ASSET_COUNT=$(curl -sS -H "Accept: application/vnd.github+json" "https://api.github.com/repos/rancher/rke2/releases/tags/${VERSION}" | jq '.assets | length')
    verify_count "${ASSET_COUNT}" "74" "RKE2 release Asset count"
}

verify_release_asset_count_k3s () {
    printf '\n==== VERIFY ASSET COUNT FOR K3S VERSION: %s: ====\n' "${VERSION}"
    debug_log "curl -sS -H \"Accept: application/vnd.github+json\" \"https://api.github.com/repos/k3s-io/k3s/releases/tags/${VERSION}\" | jq '.assets | length'"
    
    ASSET_COUNT=$(curl -sS -H "Accept: application/vnd.github+json" "https://api.github.com/repos/k3s-io/k3s/releases/tags/${VERSION}" | jq '.assets | length')
    verify_count "${ASSET_COUNT}" "16" "K3S release Asset count"
}

verify_asset_count_rke2_packaging () {
     # expect 60 cause 2 assets are the zipped and tar.gz source code
    EXPECTED_ASSETS_COUNT="${EXPECTED_ASSETS_COUNT:-60}"

    printf '\n==== VERIFY RKE2 PACKAGING ASSETS FOR RKE2 VERSION: %s ====\n' "${VERSION}"

    JSON=$(
      curl -sS -H "Accept: application/vnd.github+json" \
        "https://api.github.com/repos/rancher/rke2-packaging/releases?per_page=100"
    )

    TAG=$(printf '%s' "$JSON" | jq -r --arg p "$VERSION" '
      [ .[] | select((.tag_name | startswith($p)) and (.tag_name | test("\\.testing\\.[0-9]+$"))) ][0].tag_name
    ')
    debug_log "TAG: ${TAG}"

    if [ -z "$TAG" ]; then
        echo "FAIL: ${VERSION}: packaging release tag not found (looking for ${VERSION}.testing)." | tee -a "${FAILURE_FILE}"
        echo "Please ensure the version is correct and the release exists." | tee -a "${FAILURE_FILE}"
    fi

    ASSET_COUNT=$(printf '%s' "$JSON" | jq --arg tag "$TAG" '
      [.[] | select(.tag_name == $tag)][0].assets | length
    ')

    verify_count "${ASSET_COUNT}" "${EXPECTED_ASSETS_COUNT}" "RKE2 packaging assets"
}

verify_rke2_packaging () {
    RKE2_PKG_FILE="rke2_pkg"
    
    printf '\n==== SELINUX RKE2 PACKAGING CHECK For Product: %s Version Prefix: %s Version Suffix: %s: ====\n' "${PRODUCT}" "${VERSION_PREFIX}" "${VERSION_SUFFIX}"

    for p in 1 2 3; do
        if echo "${VERSION_PREFIX}" | grep -q "rc"; then
            debug_log "curl -s -H \"Accept: application/vnd.github+json\" https://api.github.com/repos/rancher/rke2-packaging/tags\?page=${p}\&per_page=100 | jq '.[].name' >> ${RKE2_PKG_FILE}"
            curl -s -H "Accept: application/vnd.github+json" https://api.github.com/repos/rancher/rke2-packaging/tags\?page="${p}"\&per_page=100 | jq '.[].name' >> "${RKE2_PKG_FILE}" 2>&1
        else
            debug_log "curl -s -H \"Accept: application/vnd.github+json\" https://api.github.com/repos/rancher/rke2-packaging/tags\?page=${p}\&per_page=100 | jq '.[].name' >> ${RKE2_PKG_FILE}"
            curl -s -H "Accept: application/vnd.github+json" https://api.github.com/repos/rancher/rke2-packaging/tags\?page="${p}"\&per_page=100 | jq '.[].name' >> "${RKE2_PKG_FILE}" 2>&1
        fi
    done

    if echo "${VERSION_PREFIX}" | grep -q "rc"; then
        debug_log "grep -c \"${VERSION_PREFIX}.*${VERSION_SUFFIX}\" \"${RKE2_PKG_FILE}\""
        CHANNEL_COUNT=$(grep -c "${VERSION_PREFIX}.*${VERSION_SUFFIX}" "${RKE2_PKG_FILE}")
        OUTPUT=$(grep "${VERSION_PREFIX}.*${VERSION_SUFFIX}" "${RKE2_PKG_FILE}")
    else
        debug_log "grep \"${VERSION_PREFIX}.*${VERSION_SUFFIX}\" \"${RKE2_PKG_FILE}\" | grep -vc \"rc\""
        CHANNEL_COUNT=$(grep "${VERSION_PREFIX}.*${VERSION_SUFFIX}" "${RKE2_PKG_FILE}" | grep -vc "rc")
        OUTPUT=$(grep "${VERSION_PREFIX}.*${VERSION_SUFFIX}" "${RKE2_PKG_FILE}" | grep -v "rc")
    fi

    debug_log "\nOutput:\n ${OUTPUT}"
    verify_count  "${CHANNEL_COUNT}" "1" "RKE2 packaging versions"
    
    rm -rf "${RKE2_PKG_FILE}"
}

set_url() { 
    if echo "${VERSION_PREFIX}" | grep -q "rc"; then
        PRIME_URL="docker://stgregistry.suse.com/rancher" 
    else 
        PRIME_URL="docker://registry.rancher.com/rancher" 
    fi
}

verify_registry_tags () {
    REGISTRY_REPOSITORY="$1"
    REGISTRY_DESCRIPTION="$2"
    REGISTRY_URL="${PRIME_URL}/${REGISTRY_REPOSITORY}"
    debug_log "skopeo --command-timeout 60s list-tags ${REGISTRY_URL}"
    if ! skopeo --command-timeout 60s list-tags "$REGISTRY_URL" > registry_tags.json; then
        echo "FAIL: ${VERSION}: cannot list ${REGISTRY_URL}." | tee -a "$FAILURE_FILE"
        return
    fi

    if ! jq -r --arg version "${VERSION_PREFIX}-${VERSION_SUFFIX}" '
      .Tags[] | select(. == $version or startswith($version + "-"))
    ' registry_tags.json > registry_matches.txt; then
        echo "FAIL: ${VERSION}: invalid tag response from ${REGISTRY_URL}." | tee -a "$FAILURE_FILE"
        return
    fi
    cat registry_matches.txt
    REGISTRY_COUNT=$(wc -l < registry_matches.txt)
    verify_count "$REGISTRY_COUNT" 1 "$REGISTRY_DESCRIPTION"
}

verify_prime_registry () {
    printf '\n==== VERIFY PRIME REGISTRY FOR Product: %s Version: %s ====\n' "$PRODUCT" "$VERSION"
    set_url
    if [ "$PRODUCT" = rke2 ]; then
        verify_registry_tags rke2-runtime "RKE2 Runtime(in prime registry)"
    fi
    verify_registry_tags "system-agent-installer-${PRODUCT}" "System Agent Installer for ${PRODUCT} (in prime registry)"
    verify_registry_tags "${PRODUCT}-upgrade" "${PRODUCT}-upgrade (in prime registry)"
}

verify_lts () {
    if [ "$DEBUG" = true ]; then
        set -- --debug
    else
        set --
    fi
    if ! python3 "$SCRIPT_DIR/release_checks_lts.py" "$VERSION" \
        --rpm-targets "${LTS_RPM_TARGETS:-slemicro/x86_64}" "$@"; then
        echo "FAIL: ${VERSION}: LTS artifact/index or RPM verification failed." | tee -a "$FAILURE_FILE"
    fi
    verify_prime_registry
}

# Main script execution starts here
VERSIONS=$(echo "${INPUT}" | tr "," "\n")
for VERSION in $VERSIONS
do
    printf "==========================================================================
        TESTING VERSION: %s 
==========================================================================\n" "${VERSION}"
    if echo "${VERSION}" | grep -q "rke2"; then
        PRODUCT="rke2"
    else
        PRODUCT="k3s"
    fi

    VERSION_PREFIX=$(echo "${VERSION}" | cut -d+ -f1)
    VERSION_SUFFIX=$(echo "${VERSION}" | cut -d+ -f2) # Values will be rke2r1/rke2r2/k3s1/k3s2
    
    echo "Version under test: ${VERSION} ; Prefix: ${VERSION_PREFIX} Suffix: ${VERSION_SUFFIX}"

    if [ "${LTS}" = "false" ]; then
        verify_system_agent_installers
        verify_upgrade_images
        verify_releases
        if [ "${PRODUCT}" = "rke2" ]; then
            verify_rke2_packaging
            verify_release_asset_count_rke2
            verify_asset_count_rke2_packaging
        else
            verify_release_asset_count_k3s
        fi
        verify_prime_registry
    else
        verify_lts
    fi

    printf "===================== DONE ==========================\n"
done

if [ -f "$FAILURE_FILE" ]; then
    printf "==========================================================================
                        FAILURE SUMMARY 
==========================================================================\n"
    cat "${FAILURE_FILE}"
    printf "===================== DONE ==========================\n"
    echo "Found failures. Exiting with status 1"
    rm -rf "${FAILURE_FILE}"
    exit 1
fi

rm -rf "${FAILURE_FILE}"
