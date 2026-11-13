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

lts_download () {
    debug_log "GET $1"
    curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
        --connect-timeout 10 --max-time 120 --retry 2 --retry-max-time 150 --output "$2" "$1" || {
        echo "FAIL: download $1" >&2
        return 1
    }
}

lts_child_url () {
    case "$2" in
        ''|/*|*[!a-zA-Z0-9._~+/-]*)
            echo "Invalid repository location: $2" >&2
            return 1
            ;;
    esac
    case "/$2/" in
        *'/../'*|*'/./'*|*'//'*)
            echo "Invalid repository location: $2" >&2
            return 1
            ;;
    esac
    printf '%s%s\n' "$1" "$2"
}

lts_verify_file () (
    printf '%s\n' "$2" | LC_ALL=C grep -Eq '^[[:xdigit:]]{64}$' || {
        echo "Missing or invalid SHA256 for $1" >&2
        return 1
    }
    printf '%s\n' "$3" | grep -Eq '^[0-9]+$' || return 1
    size=$(wc -c < "$1") || return 1
    if [ "$size" -ne "$3" ]; then
        echo "Size mismatch: $1" >&2
        return 1
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        digest=$(sha256sum "$1") || return 1
    else
        digest=$(shasum -a 256 "$1") || return 1
    fi
    expected=$(printf '%s' "$2" | tr 'A-F' 'a-f')
    if [ "${digest%% *}" != "$expected" ]; then
        echo "SHA256 mismatch: $1" >&2
        return 1
    fi
)

lts_verify_index () (
    base="https://prime.ribs.rancher.io/rke2/$(printf '%s' "$VERSION" | sed 's/+/%2B/')/"
    raw_base="https://prime.ribs.rancher.io/rke2/$VERSION/"
    lower_base=$(printf '%s' "$base" | sed 's/%2B/%2b/')
    lts_download "$1" lts/index.html || return 1
    xmllint --html --recover --nonet --xpath \
        "//a[starts-with(@href, '$base') or starts-with(@href, '$raw_base') or starts-with(@href, '$lower_base')]/@href" \
        lts/index.html > lts/links 2>/dev/null || {
        echo "Missing artifact links for $VERSION" >&2
        return 1
    }
    tr '"' '\n' < lts/links | sed 's/%2[bB]/+/g' | awk -v prefix="$raw_base" '
        index($0, prefix) == 1 {
            name = substr($0, length(prefix) + 1)
            if (name ~ /^[[:alnum:]_.+-]+$/) print name
        }
    ' | sort -u > lts/artifacts

    for arch in amd64 arm64; do
        manifest="sha256sum-$arch.txt"
        if ! grep -Fxq "$manifest" lts/artifacts; then
            echo "Missing $manifest" >&2
            return 1
        fi
        lts_download "$base$manifest" "lts/$manifest" || return 1
        LC_ALL=C awk '
            NF != 2 || length($1) != 64 || $1 ~ /[^[:xdigit:]]/ { exit 1 }
            {
                name = $2
                sub(/^\*/, "", name)
                if (name !~ /^[[:alnum:]_.+-]+$/ || seen[name]++) exit 1
                print name
                count++
            }
            END { if (!count) exit 1 }
        ' "lts/$manifest" > lts/manifest-files || {
            echo "Invalid or empty $manifest" >&2
            return 1
        }
        for required in "rke2.linux-$arch" "rke2.linux-$arch.tar.gz" "rke2-images.linux-$arch.tar.zst"; do
            if ! grep -Fxq "$required" lts/manifest-files; then
                echo "Missing $required in $manifest" >&2
                return 1
            fi
        done
        while IFS= read -r name; do
            if ! grep -Fxq "$name" lts/artifacts; then
                echo "Not indexed: $name" >&2
                return 1
            fi
        done < lts/manifest-files
    done
    echo "PASS: $VERSION: $(wc -l < lts/artifacts) indexed artifacts; amd64/arm64 manifests match (tarball payloads not downloaded)."
)

lts_repository () (
    base=$1
    directory=$2
    mkdir -p "$directory" || return 1
    lts_download "${base}repodata/repomd.xml" "$directory/repomd.xml" || return 1
    lts_download "${base}repodata/repomd.xml.asc" "$directory/repomd.xml.asc" || return 1
    gpgv --homedir "$WORK_DIR/lts" --keyring "$WORK_DIR/lts/keyring.gpg" \
        "$directory/repomd.xml.asc" "$directory/repomd.xml" || return 1

    primary="/*[local-name()='repomd']/*[local-name()='data'][@type='primary']"
    count=$(xmllint --nonet --xpath "count($primary)" "$directory/repomd.xml") || return 1
    if [ "$count" != 1 ]; then
        echo "Expected one primary metadata entry: $base" >&2
        return 1
    fi
    location=$(xmllint --nonet --xpath "string($primary/*[local-name()='location']/@href)" "$directory/repomd.xml") || return 1
    checksum=$(xmllint --nonet --xpath "string($primary/*[local-name()='checksum'][@type='sha256'])" "$directory/repomd.xml") || return 1
    size=$(xmllint --nonet --xpath "string($primary/*[local-name()='size'])" "$directory/repomd.xml") || return 1
    url=$(lts_child_url "$base" "$location") || return 1
    lts_download "$url" "$directory/primary.xml.gz" || return 1
    lts_verify_file "$directory/primary.xml.gz" "$checksum" "$size" || return 1
    gzip -dc "$directory/primary.xml.gz" > "$directory/primary.xml" || return 1
    xmllint --nonet --noout "$directory/primary.xml" || return 1
    echo "PASS: signed repository and primary metadata: $base"
)

lts_rpm () (
    base=$1
    directory=$2
    package=$3
    count=$(xmllint --nonet --xpath "count($package)" "$directory/primary.xml") || return 1
    if [ "$count" != 1 ]; then
        echo "Expected one matching RPM in $base: $package (found $count)" >&2
        return 1
    fi
    location=$(xmllint --nonet --xpath "string($package/*[local-name()='location']/@href)" "$directory/primary.xml") || return 1
    checksum=$(xmllint --nonet --xpath "string($package/*[local-name()='checksum'][@type='sha256'])" "$directory/primary.xml") || return 1
    size=$(xmllint --nonet --xpath "string($package/*[local-name()='size']/@package)" "$directory/primary.xml") || return 1
    url=$(lts_child_url "$base" "$location") || return 1
    lts_download "$url" "$directory/package.rpm" || return 1
    lts_verify_file "$directory/package.rpm" "$checksum" "$size" || return 1
    rm -f "$directory/package.rpm" || return 1
    echo "PASS: RPM size/SHA256: $url"
)

verify_lts_artifacts () (
    printf '%s\n' "$VERSION" | grep -Eq '^v1\.[0-9]+\.[0-9]+(-rc[0-9]+)?\+rke2r[0-9]+$' || {
        echo "Invalid RKE2 LTS release tag: $VERSION" >&2
        return 1
    }
    targets=${LTS_RPM_TARGETS:-slemicro/x86_64}
    target_pattern='(slemicro|microos|centos/(7|8|9|10))/(x86_64|aarch64)'
    printf '%s\n' "$targets" | grep -Eq "^$target_pattern(,$target_pattern)*$" || {
        echo "Invalid LTS_RPM_TARGETS: $targets" >&2
        return 1
    }
    for tool in curl xmllint gpg gpgv gzip; do
        if ! command -v "$tool" >/dev/null 2>&1; then
            echo "Missing dependency: $tool" >&2
            return 1
        fi
    done

    host=https://rpm.rancher.io
    channel=latest
    index=https://prime.ribs.rancher.io/
    case "$VERSION" in
        *-rc*)
            host=https://rpm-testing.rancher.io
            channel=testing
            index=https://prime.ribs.rancher.io/index-prerelease.html
            ;;
    esac
    minor=$(printf '%s' "${VERSION#v}" | cut -d. -f1,2)
    rpm_version=$(printf '%s' "${VERSION#v}" | tr '+-' '~~')
    mkdir -m 700 -p lts || return 1
    lts_verify_index "$index" || return 1
    lts_download "$host/public.key" lts/public.key || return 1
    gpg --homedir "$WORK_DIR/lts" --batch --yes --dearmor --output lts/keyring.gpg lts/public.key || return 1
    printf '%s\n' "$targets" | tr ',' '\n' | sort -u > lts/targets
    : > lts/common-checked

    while IFS= read -r target; do
        distro=${target%/*}
        arch=${target##*/}
        base="$host/rke2/$channel/$minor/$target/prime/"
        lts_repository "$base" lts/repo || return 1
        for name in rke2-agent rke2-common rke2-server; do
            package="/*[local-name()='metadata']/*[local-name()='package'][*[local-name()='name']='$name' and *[local-name()='arch']='$arch' and *[local-name()='version']/@ver='$rpm_version']"
            lts_rpm "$base" lts/repo "$package" || return 1
        done
        if grep -Fxq "$distro" lts/common-checked; then
            continue
        fi
        common="$host/rke2/$channel/common/$distro/noarch/"
        lts_repository "$common" lts/common || return 1
        policies="/*[local-name()='metadata']/*[local-name()='package'][*[local-name()='name']='rke2-selinux' and *[local-name()='arch']='noarch']"
        # Select the newest-built policy; dependency solving requires an install test.
        latest="(${policies}[not(*[local-name()='time']/@build < $policies/*[local-name()='time']/@build)])[1]"
        lts_rpm "$common" lts/common "$latest" || return 1
        printf '%s\n' "$distro" >> lts/common-checked
    done < lts/targets
    echo "PASS: requested LTS artifact checks. Installation and tarball payload integrity were not tested."
)

verify_lts () {
    if ! verify_lts_artifacts; then
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
