#!/usr/bin/env bash

# Test script for rke2-killall.sh or k3s-killall.sh

# Documentation:
# If arg -mount is sent true:
# DATA-DIR  Will be previously mounted => CHECK PRESENCE OF DIRECT MOUNTS => Should be present.

# If arg -mount is sent false:
# DATA-DIR  Will not be previously mounted => CHECK PRESENCE OF DIRECT MOUNTS => Should be empty.

# Set colors for output formatting
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# run all commands as user root
if [ "$(id -u)" -ne 0 ]; then
    exec sudo bash "$0" "$@"
fi

# Global variables.
declare -g PRODUCT PRODUCT_DATA_DIR MOUNT
declare -g tests_total=0 tests_passed=0
declare -a SERVICE_NAMES

# Test 1: Check if product data dir should be mounted to update test requirements.
test_data_dir_mount() {
    echo -e "\n${YELLOW}1- Testing if product data dir is $([ "$MOUNT" == "true" ] && echo "mounted" || echo "not mounted"):${NC}"

    # Check mounts.
    direct_product_mounts=$(awk -v dir="$PRODUCT_DATA_DIR" '$2 ~ dir {print $2}' /proc/mounts)
    direct_self_product_mounts=$(awk -v dir="$PRODUCT_DATA_DIR" '$2 ~ dir {print $2}' /proc/self/mounts)

    tests_total=$((tests_total + 1))
    if [ "$MOUNT" == "true" ]; then
        echo "Product data dir: ${PRODUCT_DATA_DIR} is previously mounted"
        # Should be mounted.
        if [ -n "$direct_product_mounts" ] || [ -n "$direct_self_product_mounts" ]; then
            check_result 0 "Product data dir is mounted"
            tests_passed=$((tests_passed + 1))
        else
            check_result 1 "Product data dir: ${PRODUCT_DATA_DIR} should be mounted but is not"
            echo -e "${YELLOW}Mounts:${NC}"
            echo "direct_product_mounts: $direct_product_mounts"
        fi
    else
        # Should not be mounted.
        if [ -z "$direct_product_mounts" ] && [ -z "$direct_self_product_mounts" ]; then
            check_result 0 "Product data dir is not mounted"
            tests_passed=$((tests_passed + 1))
        else
            check_result 1 "Product data dir should not be mounted but it is"
            echo -e "${YELLOW}Mounts:${NC}"
            echo -e "Direct mounts: \n$direct_product_mounts"
        fi
    fi
}

# Test 2: Check if services are stopped.
test_services_stopped() {
    echo -e "\n${YELLOW}2- Testing if services are stopped:${NC}"
    tests_total=$((tests_total + 1))

    test_pass=true
    for service in "${SERVICE_NAMES[@]}"; do
        status=$(systemctl is-active "$service" 2>/dev/null)
        if [ "$status" == "active" ]; then
            check_result 1 "Service $service is still running" "Service should be stopped but is active"
            test_pass=false
        else
            check_result 0 "Service $service is not running"
        fi
    done

    if [ "$test_pass" = true ]; then
        tests_passed=$((tests_passed + 1))
    fi
}

# Test 3: Check if important directories are removed.
test_directories_removed() {
    echo -e "\n${YELLOW}3- Testing if important directories are removed:${NC}"

    directories=(/var/lib/cni/ /run/netns/cni-*)

    if [ "$PRODUCT" == "rke2" ]; then
        directories+=(
        "/var/log/pods"
        "/var/log/containers"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/etcd.yaml"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/kube-apiserver.yaml"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/kube-controller-manager.yaml"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/cloud-controller-manager.yaml"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/kube-scheduler.yaml"
        "$PRODUCT_DATA_DIR/agent/pod-manifests/kube-proxy.yaml"
        )
    fi

    tests_total=$((tests_total + 1))
    test_pass=true
    for dir in "${directories[@]}"; do
        if [ -e "$dir" ]; then
            test_pass=false
            break
        fi
    done

    if [ "${test_pass}" = true ]; then
      check_result 0 "Dirs properly removed"
      printf '%s\n' "${directories[@]}"
       tests_passed=$((tests_passed + 1))
    else
          check_result 1 "Files or directory was not removed"
          printf '%s\n' "${directories[@]}"
    fi
}

# Test 4: Check if network interfaces are removed.
test_network_interfaces() {
    echo -e "\n${YELLOW}4- Testing if network interfaces are removed:${NC}"

    interfaces=(
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

    tests_total=$((tests_total + 1))
    test_pass=true
    for iface in "${interfaces[@]}"; do
        if ip link show "$iface" >/dev/null 2>&1; then
            test_pass=false
        fi
    done

    if [ "${test_pass}" = true ]; then
              check_result 0 "Network interfaces properly removed"
              printf '%s\n' "${interfaces[@]}"
            tests_passed=$((tests_passed + 1))
    else
            check_result 1 "Network interface **$iface** still exists"
            return
    fi
}

# Test 5: Check if there are any interfaces with 'master cni0'.
test_master_cni0() {
    echo -e "\n${YELLOW}5- Testing if there are any interfaces with 'master cni0':${NC}"
    tests_total=$((tests_total + 1))
    test_pass=true

    master_cni0_interfaces=$(ip link show 2>/dev/null | grep -q 'cni0:')
    if [ -n "$master_cni0_interfaces" ]; then
        test_pass=false
        check_result 1 "Interfaces with 'master cni0' still exist" "Found interfaces: ${master_cni0_interfaces}"
    else
        check_result 0 "No interfaces with 'master cni0' found"
    fi  

    if [ "$test_pass" = true ]; then
        tests_passed=$((tests_passed + 1))
    fi
}

# Test 6: Check if CNI-related iptables rules are removed.
test_iptables_rules() {
    echo -e "\n${YELLOW}6- Testing if CNI-related iptables rules are removed:${NC}"
    tests_total=$((tests_total + 1))

    test_pass=true

    # IPv4 checks.
    if iptables-save 2>/dev/null | grep -qE 'KUBE-|CNI-|cali-|cali:|CILIUM_|multus|multus-cni|flannel'; then
        cni_rules=$(iptables-save 2>/dev/null | grep -E 'KUBE-|CNI-|cali-|cali:|CILIUM_|multus|multus-cni|flannel' | head -5)
        check_result 1 "IPv4 iptables rules exist" "Found: ${cni_rules}..."
        test_pass=false
    else
        check_result 0 "No IPv4 iptables rules found for current CNI"
    fi

    # IPv6 check.
    if ip6tables-save 2>/dev/null | grep -qE 'KUBE-|CNI-|cali-|cali:|CILIUM_|multus|multus-cni|flannel'; then
        cni_rules_v6=$(ip6tables-save 2>/dev/null | grep -E 'KUBE-|CNI-|cali-|cali:|CILIUM_|multus|multus-cni|flannel' | head -5)
        check_result 1 "IPv6 ip6tables rules exist" "Found: ${cni_rules_v6}..."
        test_pass=false
    else
        check_result 0 "No IPv6 ip6tables rules found for current CNI"
    fi

    if [ "$test_pass" = true ]; then
        tests_passed=$((tests_passed + 1))
    fi
}

# Test 7: Check if containerd-shim processes are stopped.
test_containerd_shim() {
    echo -e "\n${YELLOW}7- Testing if containerd-shim processes are stopped:${NC}"

    tests_total=$((tests_total + 1))
    if pgrep -f "${PRODUCT_DATA_DIR}/data/.*/bin/containerd-shim" >/dev/null; then
        shim_processes=$(pgrep -a -f "${PRODUCT_DATA_DIR}/data/.*/bin/containerd-shim" | head -3)
        check_result 1 "containerd-shim processes for the product still running" "Found processes (showing first 3): ${shim_processes}..."
    else
        check_result 0 "No containerd-shim processes found"
        tests_passed=$((tests_passed + 1))
    fi
}

# Test 9: Check umounted dirs.
test_umounted_dirs() {
    echo -e "\n${YELLOW}8- Testing if umounted dirs are not mounted yet:${NC}"

    directories=(
    '/run/k3s'
    '/var/lib/kubelet/pods'
    '/run/netns/cni-'
    )

    if [ "$PRODUCT" == "k3s" ]; then
        directories+=('/var/lib/kubelet/plugins')
    fi

    tests_total=$((tests_total + 1))
    test_pass=true
    for dir in "${directories[@]}"; do
        mounts=$(awk -v dir="${dir}" '$2 ~ dir {print $2}' /proc/mounts)
        self_mounts=$(awk -v dir="${dir}" '$2 ~ dir {print $2}' /proc/self/mounts)

        if [ -n "$mounts" ] || [ -n "${self_mounts}" ]; then
            check_result 1 "mounts found for ${dir}" "mounts: ${mounts} ${self_mounts}"
            test_pass=false
        fi
    done

    if [ "$test_pass" = true ]; then
        check_result 0 "No mounts found for specified directories"
        tests_passed=$((tests_passed + 1))
    fi
}

# Check if a test passed or failed.
check_result() {
    result="$1"
    if [ "$result" -eq 0 ]; then
        echo -e "${GREEN}PASS${NC}: $2"
        return 0
    else
        echo -e "${RED}FAIL${NC}: $2 - $3"
        return 0
    fi
}

# Search for which product is installed (rke2 or k3s).
get_product() {
    if systemctl list-unit-files | grep -q rke2; then
        PRODUCT="rke2"
        PRODUCT_DATA_DIR="/var/lib/rancher/rke2"
        SERVICE_NAMES=("rke2-server" "rke2-agent")
    elif systemctl list-unit-files | grep -q k3s; then
        PRODUCT="k3s"
        PRODUCT_DATA_DIR="/var/lib/rancher/k3s"
        SERVICE_NAMES=("k3s" "k3s-agent")
    else
        echo -e "${YELLOW}Warning: Could not detect product type."
        return 1
    fi
}

# Print test summary
print_summary() {
    echo -e "Total tests: $tests_total"
    echo -e "Tests passed: ${GREEN}$tests_passed${NC}"
    echo -e "Tests failed: ${RED}$((tests_total - tests_passed))${NC}"

    if [ "$tests_passed" -eq "$tests_total" ]; then
        echo -e "\n${GREEN}All killall operations were successful!${NC}"
        echo "All killall operations were successful!" > /var/tmp/killall_result.log
        exit 0
    else
        echo -e "\n${RED}Some killall operations failed. Please see details above.${NC}"
        echo "Some killall operations failed. Please see details above." > /var/tmp/killall_result.log
        exit 0
    fi
}

# cli mount flag.
for arg in "$@"; do
    case $arg in
        -mount=*)
            MOUNT="${arg#*=}"
            shift
            ;;
        -mount)
            MOUNT="$2"
            shift 2
            ;;
    esac
done

main() {
    if [ -z "$MOUNT" ]; then
        echo -e "${RED}Error: No mount flag specified. Use '-mount true' or '-mount false'.${NC}"
        exit 1
    fi

    if [  "$MOUNT" != "true" ] && [ "$MOUNT" != "false" ]; then
        echo -e "${RED}Error: Invalid mount flag. Use '-mount true' or '-mount false'.${NC}"
        exit 1
    fi

    if ! get_product; then
        echo -e "${RED}Error: Failed to detect product type${NC}"
        exit 1
    fi

    # Run all tests!
    # Test 1
    test_data_dir_mount

    # Test 2
    test_services_stopped

    # Test 3
    test_directories_removed

    # Test 4
    test_network_interfaces

    # Test 5
    test_master_cni0

    # Test 6
    test_iptables_rules

    # Test 7
    test_containerd_shim

    # Test 8
    test_umounted_dirs

    # Print test summary.
    print_summary
}
main
