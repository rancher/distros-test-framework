#!/usr/bin/env bash

# Test script for rke2-killall.sh or k3s-killall.sh
# Usage: ./kill-all-test.sh true ( means data-dir is previously mounted ) or ./kill-all-test.sh false ( means data-dir is not previously mounted )

# Documentation:
# If arg is sent true:
# DATA-DIR  Will be mounted => CHECK PRESENCE OF DIRECT MOUNTS => Should be present.
# If arg is sent false:
# DATA-DIR  Will not be mounted => CHECK PRESENCE OF DIRECT MOUNTS => Should be empty.

# Set colors for output formatting.
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'

# running as root if not yet.
if [ "$(id -u)" -ne 0 ]; then
    exec sudo bash "$0" "$@"
fi

# Global variables.
declare -g PRODUCT PRODUCT_DATA_DIR
declare -g tests_total=0 tests_passed=0
declare -a SERVICE_NAMES

# Check if a test passed or failed.
check_result() {
    local result="$1"
    if [ "$result" -eq 0 ]; then
        echo -e "${GREEN}PASS: $2"
        return 0
    else
        echo -e "${RED}FAIL: $2 - $3"
        return 1
    fi
}

# Search for which product is installed (rke2 or k3s).
get_product() {
    if systemctl list-unit-files | grep -q rke2; then
        PRODUCT="rke2"
    elif systemctl list-unit-files | grep -q k3s; then
        PRODUCT="k3s"
    else
        echo -e "${YELLOW}Warning: Could not detect product type."
        return 1
    fi

    if [ "$PRODUCT" == "rke2" ]; then
        PRODUCT_DATA_DIR="/var/lib/rancher/rke2"
        SERVICE_NAMES=("rke2-server" "rke2-agent")
    else
        PRODUCT_DATA_DIR="/var/lib/rancher/k3s"
        SERVICE_NAMES=("k3s" "k3s-agent")
    fi
}

# Test 1: Check if product data dir is previously user mounted to update test requirements.
test_data_dir_mount() {
    local mount_flag="$1"
    echo -e "\n${YELLOW}Testing if product data dir is $([ "$mount_flag" == "true" ] && echo "mounted" || echo "not mounted"):"


#        mount --bind "$PRODUCT_DATA_DIR/server" "$PRODUCT_DATA_DIR/server"
#        mount --bind "$PRODUCT_DATA_DIR/agent" "$PRODUCT_DATA_DIR/agent"



    # Check mounts.
    direct_product_mounts=$(awk -v dir="$PRODUCT_DATA_DIR" '$2 ~ dir {print $2}' /proc/mounts)
    direct_self_product_mounts=$(awk -v dir="$PRODUCT_DATA_DIR" '$2 ~ dir {print $2}' /proc/self/mounts)

    tests_total=$((tests_total + 1))
    if [ "$mount_flag" == "true" ]; then
         # Should be mounted! If it is not then fail the test.
        if [ -n "$direct_product_mounts" ] || [ -n "$direct_self_product_mounts" ]; then
            echo -e "${YELLOW}Mounted directories:"
            echo "direct_product_mounts: $direct_product_mounts"
            check_result 0 "PRODUCT_DATA_DIR: $PRODUCT_DATA_DIR  is mounted"
            tests_passed=$((tests_passed + 1))
        else
            check_result 1 "PRODUCT_DATA_DIR: $PRODUCT_DATA_DIR should be mounted but is not"
        fi
    else
        # Should not be mounted! If it is then fail the test.
        if [ -z "$direct_product_mounts" ] && [ -z "$direct_self_product_mounts" ]; then
            check_result 0 "PRODUCT_DATA_DIR: $PRODUCT_DATA_DIR is not mounted"
            tests_passed=$((tests_passed + 1))
        else
            check_result 1  "PRODUCT_DATA_DIR: $PRODUCT_DATA_DIR should not be mounted but it is"
            echo -e "${YELLOW}Mounted directories:"
            echo "direct_product_mounts: $direct_product_mounts"
        fi
    fi
}

# Test 2: Check if services are stopped.
test_services_stopped() {
    echo -e "\n${YELLOW}Testing if services are stopped:"

    for service in "${SERVICE_NAMES[@]}"; do
        tests_total=$((tests_total + 1))
        status=$(systemctl is-active "$service" 2>/dev/null)

        if [ "$status" == "active" ]; then
            check_result 1 "Service $service is still running" "Service should be stopped but is active"
        else
            check_result 0 "Service $service is not running"
            tests_passed=$((tests_passed + 1))
        fi
    done
}

# Test 3: Check if important directories are removed.
test_directories_removed() {
    echo -e "\n${YELLOW}Testing if important directories are removed:"

    directories=(
        "/var/lib/kubelet/pods"
        "/var/log/pods"
        "/var/log/containers"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/etcd.yaml"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/kube-apiserver.yaml"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/kube-controller-manager.yaml"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/cloud-controller-manager.yaml"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/kube-scheduler.yaml"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/kube-proxy.yaml"
    )

    for dir in "${directories[@]}"; do
        tests_total=$((tests_total + 1))

        if [ -e "$dir" ]; then
            check_result 1 "Directory/file $dir still exists" "File or directory was not removed"
        else
            check_result 0 "Directory/file $dir properly removed"
            tests_passed=$((tests_passed + 1))
        fi
    done

    tests_total=$((tests_total + 1))
    if ls /run/netns/cni-* >/dev/null 2>&1; then
        cni_files=$(ls /run/netns/cni-* 2>/dev/null)
        check_result 1 "CNI network namespaces still exist" "Found: ${cni_files}"
    else
        check_result 0 "CNI network namespaces properly removed"
        tests_passed=$((tests_passed + 1))
    fi
}

# Test 4: Check if network interfaces are removed.
test_network_interfaces() {
    echo -e "\n${YELLOW}Testing if network interfaces are removed:"

    interfaces=(
        "cni0"
        "flannel.1"
        "flannel.4096"
        "flannel-v6.1"
        "flannel-v6.4096"
        "flannel-wg"
        "flannel-wg-v6"
        "vxlan.calico"
        "vxlan-v6.calico"
        "cilium_vxlan"
        "cilium_net"
        "cilium_wg0"
        "kube-ipvs0"
        "nodelocaldns"
    )

    for iface in "${interfaces[@]}"; do
        tests_total=$((tests_total + 1))
        if ip link show "$iface" >/dev/null 2>&1; then
            check_result 1 "Network interface $iface still exists" "Interface should be deleted but is still present"
        else
            check_result 0 "Network interface $iface properly removed"
            tests_passed=$((tests_passed + 1))
        fi
    done
}

# Test 5: Check if there are any interfaces with 'master cni0'.
test_master_cni0() {
    echo -e "\n${YELLOW}Testing if there are any interfaces with 'master cni0':"

    tests_total=$((tests_total + 1))
    if ip link show 2>/dev/null | grep -q 'master cni0'; then
        master_cni0_interfaces=$(ip link show 2>/dev/null | grep 'master cni0' | awk -F': ' '{print $2}' | cut -d'@' -f1)
        check_result 1 "Interfaces with 'master cni0' still exist" "Found interfaces: ${master_cni0_interfaces}"
    else
        check_result 0 "No interfaces with 'master cni0' found (this is good)"
        tests_passed=$((tests_passed + 1))
    fi
}

# Test 6: Check if CNI-related iptables rules are removed.
test_iptables_rules() {
    echo -e "\n${YELLOW}Testing if CNI-related iptables rules are removed:"
    tests_total=$((tests_total + 1))

    if iptables-save | grep -qE 'KUBE-|CNI-|cali-|cali:|CILIUM_|flannel'; then
        cni_rules=$(iptables-save | grep -E 'KUBE-|CNI-|cali-|cali:|CILIUM_|flannel' | head -5)
        check_result 1 "IPv4 iptables rules exist" "Found: ${cni_rules}..."
    else
        check_result 0 "No IPv4 iptables rules found for current CNI"
        tests_passed=$((tests_passed + 1))
    fi

    if ip6tables-save | grep -qE 'KUBE-|CNI-|cali-|cali:|CILIUM_|flannel'; then
        cni_rules_v6=$(ip6tables-save | grep -E 'KUBE-|CNI-|cali-|cali:|CILIUM_|flannel' | head -5)
        check_result 1 "IPv6 ip6tables rules exist" "Found: ${cni_rules_v6}..."
    else
        check_result 0 "No IPv6 ip6tables rules found for current CNI"
        tests_passed=$((tests_passed + 1))
    fi
}

# Test 7: Check if CNI directory is removed.
test_cni_directory() {
    echo -e "\n${YELLOW}Testing if CNI directory is removed:"

    tests_total=$((tests_total + 1))
    if [ -d "/var/lib/cni/" ]; then
        cni_contents=$(find /var/lib/cni/ -type f -exec ls -l {} + 2>/dev/null | head -5)
        check_result 1 "CNI directory /var/lib/cni/ still exists" "Directory contains: ${cni_contents}..."
    else
        check_result 0 "CNI directory /var/lib/cni/ properly removed"
        tests_passed=$((tests_passed + 1))
    fi
}

# Test 8: Check if containerd-shim processes are stopped.
test_containerd_shim() {
    echo -e "\n${YELLOW}Testing if containerd-shim processes are stopped:"

    tests_total=$((tests_total + 1))
   if pgrep -f "${PRODUCT_DATA_DIR}/data/.*/bin/containerd-shim" >/dev/null; then
        shim_processes=$(pgrep -af "${PRODUCT_DATA_DIR}/data/.*/bin/containerd-shim" | head -3)
        check_result 1 "containerd-shim processes for the product still running" "Found processes (showing first 3): ${shim_processes}..."
    else
        check_result 0 "No containerd-shim processes for the product found"
        tests_passed=$((tests_passed + 1))
    fi
}

# Print test summary
print_summary() {
    echo -e "\n${YELLOW}Test Summary:"
    echo "=========================================="
    echo -e "Total tests: $tests_total"
    echo -e "Tests passed: ${GREEN}$tests_passed"
    echo -e "Tests failed: ${RED}$((tests_total - tests_passed))"

    if [ "$tests_passed" -eq "$tests_total" ]; then
        echo -e "\n${GREEN}All cleanup operations were successful!"
        exit 0
    else
        echo -e "\n${RED}Some cleanup operations failed. Please check the details above."
        exit 1
    fi
}

main() {
    if [[ $# -lt 1 ]] || [[ ! "$1" =~ ^(true|false)$ ]]; then
        echo -e "${RED}Error: Invalid argument"
        echo "Usage: $0 <true|false>"
        exit 1
    fi

    if ! get_product; then
        echo -e "${RED}Error: Failed to detect product type"
        exit 1
    fi

    # Run all tests
    test_data_dir_mount "$1"
    test_services_stopped
    test_directories_removed
    test_network_interfaces
    test_master_cni0
    test_iptables_rules
    test_cni_directory
    test_containerd_shim

    # Print test summary
    print_summary
}
main "$1"


