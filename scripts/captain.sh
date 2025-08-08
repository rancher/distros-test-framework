#!/bin/sh

while getopts v:p:dh OPTION
do 
    case "${OPTION}"
        in
        v) INPUT=${OPTARG};;
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

debug_log () {
    if [ "${DEBUG}" = true ]; then
        echo "$ $1"
    fi
}

verify_count () {
    # $1 count $2 expected_count $3 log item we are checking
    if [ "$1" -eq "$2" ] || [ "$1" -gt "$2" ]; then
        echo "PASS: $3 Count is $1."
    else
        echo "FAIL: Not found enough $3. Expected count $2 but got $1."
    fi
}

verify_system_agent_installers () {
    # $1 product $2 version_prefix (Ex: v1.31.2-rc4) $3 version_suffix (Ex: rke2r1 or k3s1)

    echo "\n==== VERIFY SYSTEM AGENT INSTALLER FOR prdt: $1 version prefix: $2  version suffix: $3: ===="

    SYSTEM_AGENT_INSTALLER_URL="https://registry.hub.docker.com/v2/repositories/rancher/system-agent-installer-$1/tags?page_size=${PAGE_SIZE}"
    SAI_OUTPUT_FILE="sys_agent_installers_${RANDOM}"
    if echo $2 | grep -q "rc"; then
        debug_log "curl -L -s \"${SYSTEM_AGENT_INSTALLER_URL}\" | jq -r \".results[].name\" | grep $2 | grep $3 | tee -a \"${SAI_OUTPUT_FILE}\""
        curl -L -s "${SYSTEM_AGENT_INSTALLER_URL}" | jq -r ".results[].name" | grep $2 | grep $3 | tee -a "${SAI_OUTPUT_FILE}"
    else
        debug_log "curl -L -s \"${SYSTEM_AGENT_INSTALLER_URL}\" | jq -r \".results[].name\" | grep $2 | grep $3 | grep -v \"rc\" | tee -a \"${SAI_OUTPUT_FILE}\""
        curl -L -s "${SYSTEM_AGENT_INSTALLER_URL}" | jq -r ".results[].name" | grep $2 | grep $3 | grep -v "rc" | tee -a "${SAI_OUTPUT_FILE}"
    fi
    COUNT=$(cat "${SAI_OUTPUT_FILE}" | wc -l)
    WINDOWS_COUNT=$(cat "${SAI_OUTPUT_FILE}" | grep "windows" | wc -l)
    LINUX_ARM_COUNT=$(cat "${SAI_OUTPUT_FILE}" | grep "linux-arm64" | wc -l)
    LINUX_AMD_COUNT=$(cat "${SAI_OUTPUT_FILE}" | grep "linux-amd64" | wc -l)
    if [ "$1" = "rke2" ]; then
        verify_count "${COUNT}" "5" "Total images"
        verify_count "${WINDOWS_COUNT}" "2" "Windows images"
        verify_count "${LINUX_ARM_COUNT}" "1" "Linux arm images"
        verify_count "${LINUX_AMD_COUNT}" "1" "linux-amd64 images"
    else
        verify_count "${COUNT}" "3" "Total images"
        verify_count "${LINUX_ARM_COUNT}" "1" "Linux arm images"
        verify_count "${LINUX_AMD_COUNT}" "1" "linux-amd64 images"
    fi
    rm -rf "${SAI_OUTPUT_FILE}"
}

verify_upgrade_images () {
    # $1 product $2 version_prefix (Ex: v1.31.2-rc4) $3 version_suffix (Ex: rke2r1 or k3s1)

    echo "\n==== VERIFY UPGRADE IMAGES FOR $1 $2: ===="

    UPGRADE_IMAGES_URL="https://registry.hub.docker.com/v2/repositories/rancher/$1-upgrade/tags?page_size=${PAGE_SIZE}"
    UPG_OUTPUT_FILE="upgrade_images_${RANDOM}"
    if echo $2 | grep -q "rc"; then
        debug_log "curl -L -s ${UPGRADE_IMAGES_URL} | jq -r \".results[].name\" | grep $2 | grep $3"
        curl -L -s "${UPGRADE_IMAGES_URL}" | jq -r ".results[].name" | grep $2 | grep $3 | tee -a "${UPG_OUTPUT_FILE}"
    else
        debug_log "curl -L -s ${UPGRADE_IMAGES_URL} | jq -r \".results[].name\" | grep $2 | grep $3 | grep -v \"rc\""
        curl -L -s "${UPGRADE_IMAGES_URL}" | jq -r ".results[].name" | grep $2 | grep $3 | grep -v "rc" | tee -a "${UPG_OUTPUT_FILE}"
    fi
    UPGRADE_COUNT=$(cat "${UPG_OUTPUT_FILE}" | wc -l)
    verify_count "${UPGRADE_COUNT}" "1" "Upgrade images"
    rm -rf "${UPG_OUTPUT_FILE}"
}

verify_releases () {
    # useless unless we can check on the asset count.
    # $1 product $2 version_prefix (Ex: v1.31.2-rc4) $3 version_suffix (Ex: rke2r1 or k3s1)

    echo "\n==== VERIFY RELEASES FOR $1 $2: ===="

    if [ "$1" = "rke2" ]; then
        RELEASES_URL="https://api.github.com/repos/rancher/rke2/releases"
    else
        RELEASES_URL="https://api.github.com/repos/k3s-io/k3s/releases"
    fi

    RELEASES_FILE="releases_${RANDOM}"
    if echo $2 | grep -q "rc"; then
        debug_log "curl -s -H \"Accept: application/vnd.github+json\" ${RELEASES_URL} | jq '.[].tag_name' | grep $2 | grep $3"
        curl -s -H "Accept: application/vnd.github+json" "${RELEASES_URL}" | jq '.[].tag_name' | grep $2 | grep $3 | tee -a "${RELEASES_FILE}"
    else
        debug_log "curl -s -H \"Accept: application/vnd.github+json\" ${RELEASES_URL} | jq '.[].tag_name' | grep $2 | grep $3 | grep -v \"rc\""
        curl -s -H "Accept: application/vnd.github+json" "${RELEASES_URL}" | jq '.[].tag_name' | grep $2 | grep $3 | grep -v "rc" | tee -a "${RELEASES_FILE}"
    fi
    RELEASES_COUNT=$(cat "${RELEASES_FILE}" | wc -l)
    verify_count "${RELEASES_COUNT}" "1" "Release versions"
    rm -rf "${RELEASES_FILE}"
}

verify_rke2_packaging () {
    # $1 product $2 version_prefix (Ex: v1.31.2-rc4) $3 version_suffix (Ex: rke2r1 or k3s1)
    echo "\n==== SELINUX RKE2 PACKAGING CHECK: $1 $2 $3: ===="
    RKE2_PKG_FILE="rke2_pkg_${RANDOM}"
    for p in {1..2}; do
        if echo $2 | grep -q "rc"; then
            debug_log "curl -s -H \"Accept: application/vnd.github+json\" https://api.github.com/repos/rancher/rke2-packaging/tags\?page\=${p}\&per_page\=100 | jq '.[].name' >> ${RKE2_PKG_FILE}"
            curl -s -H "Accept: application/vnd.github+json" https://api.github.com/repos/rancher/rke2-packaging/tags\?page\="${p}"\&per_page\=100 | jq '.[].name' >> "${RKE2_PKG_FILE}" 2>&1
        else
            debug_log "curl -s -H \"Accept: application/vnd.github+json\" https://api.github.com/repos/rancher/rke2-packaging/tags\?page\=${p}\&per_page\=100 | jq '.[].name' >> ${RKE2_PKG_FILE}"
            curl -s -H "Accept: application/vnd.github+json" https://api.github.com/repos/rancher/rke2-packaging/tags\?page\="${p}"\&per_page\=100 | jq '.[].name' >> "${RKE2_PKG_FILE}" 2>&1
        fi
    done
    CHANNEL_COUNT=$(cat "${RKE2_PKG_FILE}" | grep $2 | grep $3 | wc -l)
    verify_count  "${CHANNEL_COUNT}" "1" "RKE2 packaging versions"
    rm -rf "${RKE2_PKG_FILE}"
}

verify_prime_registry () {
    # $1 product $2 version_prefix (Ex: v1.31.2-rc4) $3 version_suffix (Ex: rke2r1 or k3s1)
    echo "\n==== VERIFY PRIME REGISTRY FOR $1 $2 $3: ===="
    SYS_AGENT_OUTFILE="sys_agent_${RANDOM}"
    RKE2_RUNTIME_OUTFILE="rke2_runtime_${RANDOM}"
    if [ "$1" = "rke2" ]; then
        SYS_AGENT_INSTALLER_URL="docker://registry.rancher.com/rancher/system-agent-installer-rke2"
        debug_log "skopeo list-tags ${SYS_AGENT_INSTALLER_URL} | grep \"$2\" | grep \"$3\" | tee -a \"${SYS_AGENT_OUTFILE}\""
        skopeo list-tags "${SYS_AGENT_INSTALLER_URL}" | grep "$2" | grep "$3" | tee -a "${SYS_AGENT_OUTFILE}"
        SYS_AGENT_COUNT=$(cat "${SYS_AGENT_OUTFILE}" | wc -l)
        verify_count "${SYS_AGENT_COUNT}" "1" "System Agent Installer for rke2(in prime registry)"

        RKE2_RUNTIME_URL="docker://registry.rancher.com/rancher/rke2-runtime"
        if echo $2 | grep -q "rc"; then
            debug_log "skopeo list-tags ${RKE2_RUNTIME_URL} | grep \"$2\" | grep \"$3\" | tee -a ${RKE2_RUNTIME_OUTFILE}"
            skopeo list-tags "${RKE2_RUNTIME_URL}" | grep "$2" | grep "$3" | tee -a "${RKE2_RUNTIME_OUTFILE}"
        else
            debug_log "skopeo list-tags ${RKE2_RUNTIME_URL} | grep \"$2\" | grep \"$3\" | grep -v rc | tee -a ${RKE2_RUNTIME_OUTFILE}"
            skopeo list-tags "${RKE2_RUNTIME_URL}" | grep "$2" | grep "$3" | grep -v rc | tee -a "${RKE2_RUNTIME_OUTFILE}"
        fi
        RKE2_RUNTIME_COUNT=$(cat "${RKE2_RUNTIME_OUTFILE}" | wc -l)
        verify_count "${RKE2_RUNTIME_COUNT}" "1" "RKE2 Runtime(in prime registry)"
    else
        SYS_AGENT_INSTALLER_URL="docker://registry.rancher.com/rancher/system-agent-installer-k3s"
        debug_log "skopeo list-tags ${SYS_AGENT_INSTALLER_URL} | grep \"$2\" | grep \"$3\" | tee -a ${SYS_AGENT_OUTFILE}"
        skopeo list-tags "${SYS_AGENT_INSTALLER_URL}" | grep "$2" | grep "$3" | tee -a "${SYS_AGENT_OUTFILE}"
        SYS_AGENT_COUNT=$(cat "${SYS_AGENT_OUTFILE}" | wc -l)
        verify_count "${SYS_AGENT_COUNT}" "1" "System Agent Installer for k3s(in prime registry)"
    fi

    rm -rf "${SYS_AGENT_OUTFILE}"
    rm -rf "${RKE2_RUNTIME_OUTFILE}"
}

# Main script execution starts here
VERSIONS=$(echo "${INPUT}" | tr "," "\n")
for i in $VERSIONS
do
    echo "==========================================================================
        TESTING VERSION: $i 
=========================================================================="
    if echo $i | grep -q "rke2"; then
        PRODUCT="rke2"
    else
        PRODUCT="k3s"
    fi
    VERSION_PREFIX=$(echo $i | cut -d+ -f1)
    VERSION_SUFFIX=$(echo $i | cut -d+ -f2) # Values will be rke2r1/rke2r2/k3s1/k3s2
    echo "Version under test: $i ; Prefix: $VERSION_PREFIX Suffix: $VERSION_SUFFIX"
    verify_system_agent_installers "${PRODUCT}" "${VERSION_PREFIX}" "${VERSION_SUFFIX}"
    verify_upgrade_images "${PRODUCT}" "${VERSION_PREFIX}" "${VERSION_SUFFIX}"
    verify_releases "${PRODUCT}" "${VERSION_PREFIX}" "${VERSION_SUFFIX}"
    if [ "${PRODUCT}" = "rke2" ]; then
        verify_rke2_packaging "${PRODUCT}" "${VERSION_PREFIX}" "${VERSION_SUFFIX}"
    fi
    verify_prime_registry "${PRODUCT}" "${VERSION_PREFIX}" "${VERSION_SUFFIX}"
    echo "===================== DONE ==========================\n"
done
