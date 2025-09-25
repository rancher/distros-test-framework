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
            -l: LTS version to test. Ex: v1.30.14-rc6+rke2r3
            -p: page size for curl commands to fetch data from docker hub or github. Default is 200.
            -h: help - usage is displayed
            "
            exit 1
            ;;
        esac
done

# Set defaults if not provided
if [ -z "${INPUT}" ]; then
    echo "Please provide the version to test using -v option."
    exit 1
fi
if [ -z "${DEBUG}" ]; then
    DEBUG="false"
fi
if [ -z "${PAGE_SIZE}" ]; then
    PAGE_SIZE=200
fi

SEED=$(date +%s)
RANDOM_INT=$(awk -v seed="${SEED}" 'BEGIN { srand(seed); print int(rand() * 100) }')
FAILURE_FILE="failure_results_${RANDOM_INT}"

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
    SAI_OUTPUT_FILE="sys_agent_installers_${RANDOM_INT}"
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
    UPG_OUTPUT_FILE="upgrade_images_${RANDOM_INT}"
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
    RELEASES_FILE="releases_${RANDOM_INT}"
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
    verify_count "${ASSET_COUNT}" "19" "K3S release Asset count"
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
    RKE2_PKG_FILE="rke2_pkg_${RANDOM_INT}"
    
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
        debug_log "grep \"${VERSION_PREFIX}.*${VERSION_SUFFIX}\" \"${RKE2_PKG_FILE}\" | grep -v \"rc\" | wc -l"
        CHANNEL_COUNT=$(grep "${VERSION_PREFIX}.*${VERSION_SUFFIX}" "${RKE2_PKG_FILE}" | grep -v "rc" | wc -l)
        OUTPUT=$(grep "${VERSION_PREFIX}.*${VERSION_SUFFIX}" "${RKE2_PKG_FILE}" | grep -v "rc")
    fi

    debug_log "\nOutput:\n ${OUTPUT}"
    verify_count  "${CHANNEL_COUNT}" "1" "RKE2 packaging versions"
    
    rm -rf "${RKE2_PKG_FILE}"
}

verify_prime_registry () {
    printf '\n==== VERIFY PRIME REGISTRY FOR Product: %s Version Prefix: %s Version Suffix: %s: ====\n' "${PRODUCT}" "${VERSION_PREFIX}" "${VERSION_SUFFIX}"

    # rke2-runtime is only for rke2 product
    if [ "${PRODUCT}" = "rke2" ]; then
        RKE2_RUNTIME_URL="docker://registry.rancher.com/rancher/rke2-runtime"
        RKE2_RUNTIME_OUTFILE="rke2_runtime_${RANDOM_INT}"

        if echo "${VERSION_PREFIX}" | grep -q "rc"; then
            debug_log "skopeo list-tags ${RKE2_RUNTIME_URL} | grep ${VERSION_PREFIX} | grep ${VERSION_SUFFIX} | tee -a ${RKE2_RUNTIME_OUTFILE}"
            skopeo list-tags "${RKE2_RUNTIME_URL}" | grep "${VERSION_PREFIX}" | grep "${VERSION_SUFFIX}" | tee -a "${RKE2_RUNTIME_OUTFILE}"
        else
            debug_log "skopeo list-tags ${RKE2_RUNTIME_URL} | grep ${VERSION_PREFIX} | grep ${VERSION_SUFFIX} | grep -v rc | tee -a ${RKE2_RUNTIME_OUTFILE}"
            skopeo list-tags "${RKE2_RUNTIME_URL}" | grep "${VERSION_PREFIX}" | grep "${VERSION_SUFFIX}" | grep -v rc | tee -a "${RKE2_RUNTIME_OUTFILE}"
        fi
    
        RKE2_RUNTIME_COUNT=$(wc -l < "${RKE2_RUNTIME_OUTFILE}")
        verify_count "${RKE2_RUNTIME_COUNT}" "4" "RKE2 Runtime (in prime registry)"
        rm -rf "${RKE2_RUNTIME_OUTFILE}"
    fi

    # Verify system-agent-installer and upgrade images in prime registry for k3s and rke2 products
    URL_ITEMS="system-agent-installer-${PRODUCT} ${PRODUCT}-upgrade"

    for ITEM in $URL_ITEMS; do
        PRIME_URL="docker://registry.rancher.com/rancher/${ITEM}"
        OUTFILE="${ITEM}_${VERSION_PREFIX}_${RANDOM_INT}"

        debug_log "skopeo list-tags ${PRIME_URL} | grep ${VERSION_PREFIX} | grep ${VERSION_SUFFIX} | tee -a ${OUTFILE}"
        skopeo list-tags "${PRIME_URL}" | grep "${VERSION_PREFIX}" | grep "${VERSION_SUFFIX}" | tee -a "${OUTFILE}"
        
        COUNT=$(wc -l < "${OUTFILE}")
        if echo "${VERSION_PREFIX}" | grep -q "rc"; then
            verify_count "${COUNT}" "0" "${ITEM} (in prime registry)"
        else
            verify_count "${COUNT}" "1" "${ITEM} (in prime registry)"
        fi

        rm -rf "${OUTFILE}"
    done
}

verify_lts () {
    LTS_OUT_FILE="lts_output_${RANDOM_INT}"
    if echo "${VERSION_PREFIX}" | grep -q "rc"; then
        LTS_URL="https://prime.ribs.rancher.io/index-prerelease.html"
    else
        LTS_URL="https://prime.ribs.rancher.io"
    fi
    curl "${LTS_URL}" > "${LTS_OUT_FILE}"
    LTS_COUNT=$(grep -cE "${VERSION_PREFIX}.*${VERSION_SUFFIX}" "${LTS_OUT_FILE}")
    verify_count "${LTS_COUNT}" "76" "LTS count check against Prime registry"
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

if [ -f "${FAILURE_FILE}" ]; then
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
