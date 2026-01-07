#!/usr/bin/env bash

# Uninstall Test Script for RKE2 and K3S.
# Tests removal of binaries, configurations, environment, system artifacts and scripts.

# Set colors for output formatting
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# run all commands as user root.
if [ "$(id -u)" -ne 0 ]; then
    exec sudo bash "$0" "$@"
fi

# Global vars.
declare -g PRODUCT PRODUCT_DATA_DIR PRODUCT_ROOT_BIN
declare -g tests_total=0 tests_passed=0

# Test-1: Verify Binary Removal.
test_binary_removal() {
    echo -e "\n${YELLOW}1- Testing if product binaries are removed:${NC}"
    tests_total=$((tests_total + 1))
    test_pass=true
    found_symlinks=()

    binary_names_deleted=(
        "kubectl"
        "crictl"
        "ctr"
    )

    if [ "$PRODUCT" == "rke2" ]; then
        binary_names_deleted+=("rke2")
    else
        binary_names_deleted+=("k3s")
    fi

    # Check for binaries in the product bin directory, if some is still found, checks if it is a symlink, if its not that a pass
    # because the uninstall script should not remove it as per https://github.com/k3s-io/k3s/blob/a897f6875ecab818346e02663d43cd83c9ee66ba/install.sh#L753.
    for binary in "${binary_names_deleted[@]}"; do
        bin_var_name=$(echo "${binary}" | tr '[:lower:]' '[:upper:]')_BIN_DIR
        if [ -n "${!bin_var_name}" ]; then
            full_path="${!bin_var_name}/${binary}"
             if [ -e "$full_path" ]; then
                 if [ -L "$full_path" ]; then
                    test_pass=false
                    found_symlinks+=("$full_path")
                    echo -e "${RED}FAIL${NC}: Binary $full_path is a symlink and was not removed."
                else
                    echo -e "${YELLOW}INFO${NC}: Binary $full_path exists but is not a symlink. Uninstall script correctly skipped it."
                fi
            fi
        fi
    done

     if [ "$test_pass" = true ]; then
        check_result 0 "All expected product symlinks removed or skipped correctly."
        tests_passed=$((tests_passed + 1))
    else
        binary_list=$(printf ", %s" "${found_symlinks[@]}")
        binary_list=${binary_list:2}
        check_result 1 "Found lingering symlinks: ${binary_list}" "Not all expected symlinks were removed."
    fi
}

# Test-2: Verify Configuration Directory Removal.
test_config_removal() {
    echo -e "\n${YELLOW}2- Testing if configuration directories are removed:${NC}"
    tests_total=$((tests_total + 1))
    test_pass=true

    config_dirs=(
        "/etc/rancher/${PRODUCT}"
        "${PRODUCT_ROOT}/share/${PRODUCT}"
        "/tmp/*_kubeconfig_*"
        "/var/lib/rancher/${PRODUCT}/agent/etc"
        "/var/tmp/*_kubeconfig_*"
    )

    if [ "$PRODUCT" == "rke2" ]; then
       config_dirs+=(
        "/etc/rancher/node"
        "/etc/cni"
        "/opt/cni/bin"
       )
    fi

    # Check for config directories with wildcards.
    for dir in "${config_dirs[@]}"; do
        if ls "$dir" &>/dev/null; then
            test_pass=false
            break
        fi
    done

    if [ "$test_pass" = true ]; then
        check_result 0 "Configuration directories removed"
        tests_passed=$((tests_passed + 1))
    else
        check_result 1 "Configuration directory $dir still exists" "Not all config directories were removed"
    fi
}

# Test-3: Verify Data and Runtime Directory Removal.
test_data_dir_removal() {
    echo -e "\n${YELLOW}3- Testing if data and runtime directories are removed:${NC}"
    tests_total=$((tests_total + 1))
    test_pass=true

    # common directories.
    data_dirs=(
        "${PRODUCT_DATA_DIR}"
        "/run/${PRODUCT}"
        "/opt/cni/bin"
    )

    # not adding /var/lib/kubelet to be checked when this var is set and is true
    # because it will be mounted test dirs inside it and it will preventing deletion as "device or resource busy"
    # so test will fail being a false negative.
    if ! run_test_file_removal_safety; then
        data_dirs+=("/var/lib/kubelet")
    fi

    if [ "$PRODUCT" == "rke2" ]; then
        data_dirs+=("/var/lib/rancher")
    else
        data_dirs+=( "/run/flannel")
    # "/var/lib/rancher/" should be empty, not deleted for k3s.
       if [ "$(find "/var/lib/rancher/" -type f | wc -l)" -eq 0 ]; then
        echo "No files found in /var/lib/rancher"
        test_pass=true
       else
        echo "Files found in /var/lib/rancher"
        test_pass=false
       fi
    fi

    for dir in "${data_dirs[@]}"; do
        if [ -d "$dir" ]; then
            test_pass=false
            break
        fi
    done

    if [ "$test_pass" = true ]; then
        check_result 0 "Data and runtime directories removed"
        tests_passed=$((tests_passed + 1))
    else
        check_result 1 "Data directory $dir still exists" "Not all data directories were removed"
    fi
}

# Test-4: Verify Service File Removal.
test_service_file_removal() {
    echo -e "\n${YELLOW}4- Testing if service files are removed:${NC}"
    tests_total=$((tests_total + 1))

    service_files=("/etc/systemd/system/${PRODUCT}")

    if [ "$PRODUCT" == "rke2" ]; then
        service_files+=(
            "/etc/systemd/system/multi-user.target.wants/rke2-server.service"
            "/etc/systemd/system/multi-user.target.wants/rke2-agent.service"
        )
    else
        service_files+=(
            "/etc/systemd/system/k3s-agent.service"
            "/etc/systemd/system/k3s.service"
            "/etc/systemd/system/multi-user.target.wants/k3s-agent.service"
            "/etc/systemd/system/multi-user.target.wants/k3s.service"
        )
    fi

    test_pass=true
    for file in "${service_files[@]}"; do
        if [ -f "$file" ]; then
            test_pass=false
            break
        fi
    done

    if [ "$test_pass" = true ]; then
        check_result 0 "Service files removed"
        tests_passed=$((tests_passed + 1))
    else
        check_result 1 "Service file $file still exists" "Not all service files were removed"
    fi
}

# Test-5: Verify IPTables Rules Removal.
test_iptables_removal() {
    echo -e "\n${YELLOW}5- Testing if IPTables rules are cleared:${NC}"
    tests_total=$((tests_total + 1))

    test_pass=true

    # IPv4.
    if iptables-save 2>/dev/null | grep -qE 'KUBE-|CNI-|cali-|cali:|CILIUM_|flannel|K3S|RKE2'; then
         cni_rules=$(iptables-save 2>/dev/null | grep -E 'KUBE-|CNI-|cali-|cali:|CILIUM_|flannel|K3S|RKE2' | head -5)
        check_result 1 "IPv4 iptables rules still exist" "Found: ${cni_rules}..."
        test_pass=false
    else
        check_result 0 "No IPv4 iptables rules found"
    fi

    # IPv6.
    if ip6tables-save 2>/dev/null | grep -qE 'KUBE-|CNI-|cali-|cali:|CILIUM_|flannel|K3S|RKE2'; then
        cni_rules_v6=$(ip6tables-save 2>/dev/null | grep -E 'KUBE-|CNI-|cali-|cali:|CILIUM_|flannel|K3S|RKE2' | head -5)
        check_result 1 "IPv6 ip6tables rules still exist" "Found: ${cni_rules_v6}..."
        test_pass=false
    else
        check_result 0 "No IPv6 ip6tables rules found"
    fi

    if [ "$test_pass" = true ]; then
        tests_passed=$((tests_passed + 1))
    fi
}

 # Test-6: Verify Fapolicyd Rules Removal.
test_fapolicyd_removal() {
    echo -e "\n${YELLOW}6- Testing if Fapolicyd rules are cleared:${NC}"
    tests_total=$((tests_total + 1))
    test_pass=true

    if ! command -v fapolicyd &>/dev/null; then
        check_result 0 "Fapolicyd not installed"
        tests_passed=$((tests_passed + 1))
        return
    fi

    if fapolicyd --list-rules | grep -qE 'KUBE-|CNI-|cali-|cali:|CILIUM_|flannel|multus|multus-cni|K3S|RKE2'; then
        check_result 1 "Fapolicyd rules still exist" "Found: ${cni_rules}..."
        test_pass=false
    else
        check_result 0 "No Fapolicyd rules found"
        tests_passed=$((tests_passed + 1))
    fi
}

#Test 7: Verify RPM Repositories Removal.
test_rpm_repo_removal() {
    echo -e "\n${YELLOW}7- Testing if RPM repositories are removed:${NC}"
    tests_total=$((tests_total + 1))

    test_pass=true

    # Check for repo files.
    if [ -f "/etc/yum.repos.d/rancher-${PRODUCT}-*.repo" ]; then
        check_result 1 "RPM repository file still exists" "Found: /etc/yum.repos.d/rancher-${PRODUCT}-*.repo"
        test_pass=false
    else
        check_result 0 "No RPM repository files found"
    fi

    # Check if SELinux packages are removed.
    if command -v rpm &>/dev/null; then
        selinux_pkg="${PRODUCT}-selinux"
        if rpm -q "$selinux_pkg" &>/dev/null; then
            check_result 1 "SELinux package still installed" "Found: $selinux_pkg"
            test_pass=false
        else
            check_result 0 "No SELinux package found"
        fi

        # Check for other product RPM packages.
        rpm_pkgs=$(rpm -qa | grep -E "${PRODUCT}-(common|agent|server)" || true)
        if [ -n "$rpm_pkgs" ]; then
            check_result 1 "Product RPM packages still installed" "Found: $rpm_pkgs"
            test_pass=false
        else
            check_result 0 "No product RPM packages found"
        fi
    fi

    if [ "$test_pass" = true ]; then
        tests_passed=$((tests_passed + 1))
    fi
}

# Test-8: Verify Environment File Removal.
test_env_file_removal() {
    echo -e "\n${YELLOW}8- Testing if product environment files are removed:${NC}"
    tests_total=$((tests_total + 1))

    product_root_install="${PRODUCT_ROOT_BIN%/*/*}"
    test_pass=true

    # Checking specific files for rke2 and k3s due to -f flag not work with wildcards.
    if [ "$PRODUCT" == "rke2" ]; then
        env_files=(
            "/etc/systemd/system/rke2-server.service.env"
            "/etc/systemd/system/rke2-agent.service.env"
            "${product_root_install}/share/rke2/rke2.env"
            "${product_root_install}/lib/systemd/system/rke2-server.env"
            "${product_root_install}/lib/systemd/system/rke2-agent.env"
            "${product_root_install}/usr/lib/systemd/system/rke2-server.env"
            "${product_root_install}/usr/lib/systemd/system/rke2-agent.env"
        )
    else
        env_files=(
            "/etc/systemd/system/k3s.service.env"
            "/etc/systemd/system/k3s-agent.service.env"
            "${product_root_install}/lib/systemd/system/k3s.service.env"
            "${product_root_install}/lib/systemd/system/k3s-agent.service.env"
            "${product_root_install}/usr/lib/systemd/system/k3s.service.env"
            "${product_root_install}/usr/lib/systemd/system/k3s-agent.service.env"
        )
    fi

    for env_file in "${env_files[@]}"; do
        if [ -f "$env_file" ]; then
            test_pass=false
            check_result 1 "Environment file still exists" "Found: $env_file"
        fi
    done

    if [ "$test_pass" = true ]; then
        check_result 0 "All product environment files removed"
        tests_passed=$((tests_passed + 1))
    fi
}

# Test-9: Verify Uninstall and Kill Script Removal.
test_scripts_removal() {
    echo -e "\n${YELLOW}9- Testing if uninstall and kill scripts are removed:${NC}"
    tests_total=$((tests_total + 1))

    script_names=("${PRODUCT_ROOT_BIN}-uninstall.sh" "${PRODUCT_ROOT_BIN}-killall.sh")
    test_pass=true
    found_scripts=()

    for script in "${script_names[@]}"; do
        if [ -f "$script" ]; then
            test_pass=false
            found_scripts+=("$script")
        fi
    done

    if [ "$test_pass" = true ]; then
        check_result 0 "All uninstall/kill scripts files removed"
        tests_passed=$((tests_passed + 1))
    else
        script_list=$(printf ", %s" "${found_scripts[@]}")
        script_list=${script_list:2}
        check_result 1 "Found scripts: ${script_list}" "Uninstall/kill scripts not removed"
    fi
}

# Test-10: Verify removal of semodule.
test_semodule_removal() {
    echo -e "\n${YELLOW}10- Testing if RKE2 semodule is removed:${NC}"
    tests_total=$((tests_total + 1))
    test_pass=true

    if command -v semodule &>/dev/null; then
         if semodule -l 2>/dev/null | grep -q "rke2"; then
            rke2_modules=$(semodule -l 2>/dev/null | grep "rke2")
            check_result 1 "RKE2 SELinux modules still exist" "Found: ${rke2_modules}"
            test_pass=false
        else
            check_result 0 "No RKE2 SELinux modules found"
        fi
    else
        echo "SELinux not installed (semodule command not found)"
        check_result 0 "SELinux not installed"
    fi

    if [ "$test_pass" = true ]; then
        tests_passed=$((tests_passed + 1))
    fi
}

## Test-11: Check file removal safety with --one-filesystem.
test_file_removal_safety() {
    echo -e "\n${YELLOW}11- Testing file removal safety with --one-filesystem:${NC}"
    tests_total=$((tests_total + 1))

    expected_files=("important.txt" "critical.txt")
    test_pass=true
    missing_files=()

    for file in "${expected_files[@]}"; do
        if [ ! -f "/mnt/fake-remote-fs/$file" ]; then
            test_pass=false
            missing_files+=("$file")
        fi
    done

    if [ "$test_pass" = true ]; then
        check_result 0 "All critical files protected by --one-file-system"
        tests_passed=$((tests_passed + 1))
    else
        file_list=$(printf ", %s" "${missing_files[@]}")
        file_list=${file_list:2}
        check_result 1 "Missing files: ${file_list}" "Files were deleted despite --one-file-system"
    fi
}

###################    Auxiliary functions    #####################
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

get_product() {
    if [ -n "$PRODUCT_FLAG" ]; then
        if [ "$PRODUCT_FLAG" == "rke2" ] || [ "$PRODUCT_FLAG" == "k3s" ]; then
            PRODUCT="$PRODUCT_FLAG"
            PRODUCT_DATA_DIR="/var/lib/rancher/$PRODUCT"

           if [ "$PRODUCT" == "rke2" ]; then
                PRODUCT_ROOT_BIN="$RKE2_BIN_DIR"
              else
                PRODUCT_ROOT_BIN="$K3S_BIN_DIR"
           fi

            echo -e "${YELLOW}Using product: ${PRODUCT}${NC}"
            return 0
        else
            echo -e "${RED}Invalid product: ${PRODUCT_FLAG}. Use 'rke2' or 'k3s'.${NC}"
            return 1
        fi
    fi
}

run_test_file_removal_safety() {
    if [ -f /etc/profile.d/env_vars.sh ]; then
        source /etc/profile.d/env_vars.sh
    fi

    if [ "${RUN_TEST_FILE_REMOVAL_SAFETY}" != "true" ]; then
        echo -e "\n${YELLOW}Skipping test 11: File removal safety with --one-filesystem.${NC}"
        return 1
    else
        echo -e "\n${YELLOW}Running test 11: File removal safety with --one-filesystem.${NC}"
        return 0
    fi
}

# Print test summary
print_summary() {
    echo -e "Total tests: $tests_total"
    echo -e "Tests passed: ${GREEN}$tests_passed${NC}"
    echo -e "Tests failed: ${RED}$((tests_total - tests_passed))${NC}"

    if [ "$tests_passed" -eq "$tests_total" ]; then
        echo -e "\n${GREEN}All uninstall operations were successful!${NC}"
        echo "All uninstall operations were successful!" >> /var/log/uninstall_result.log
        exit 0
    else
        echo -e "\n${RED}Some uninstall operations failed. Please check the details above.${NC}"
        echo "Some uninstall operations failed. Please check the details above." >> /var/log/uninstall_result.log
        exit 0
    fi
}

# cli product flag.
for arg in "$@"; do
    case $arg in
        -p=*)
            PRODUCT_FLAG="${arg#*=}"
            shift
            ;;
        -p)
            PRODUCT_FLAG="$2"
            shift 2
            ;;
    esac
done

main() {
    # Source the path for environment variables added by golang code.
    if [ -f "/etc/profile.d/bin_paths.sh" ]; then
        . /etc/profile.d/bin_paths.sh
        cat /etc/profile.d/bin_paths.sh
    fi

    if [ -z "$PRODUCT_FLAG" ]; then
        echo -e "${RED}Error: No product specified. Use 'rke2' or 'k3s'.${NC}"
        exit 1
    fi

    if [ "$PRODUCT_FLAG" != "rke2" ] && [ "$PRODUCT_FLAG" != "k3s" ]; then
        echo -e "${RED}Error: Invalid product specified. Use 'rke2' or 'k3s'.${NC}"
        exit 1
    fi

    if ! get_product; then
        echo -e "${RED}Error: Failed to detect product${NC}"
        exit 1
    fi

    # Run tests!
    # Test 1
    test_binary_removal

    # Test 2
    test_config_removal

    # Test 3
    test_data_dir_removal

    # Test 4
    test_service_file_removal

    # Test 5
    test_iptables_removal

    # Test 6
    test_fapolicyd_removal

    # Test 7
    test_rpm_repo_removal

    # Test 8
    test_env_file_removal

    # Test 9
    test_scripts_removal

    # Test 10
    test_semodule_removal

    #Only support RKE2 for now.
    if run_test_file_removal_safety && [ "$PRODUCT_FLAG" == "rke2" ]; then
      # Test 11
      test_file_removal_safety
    fi

    # Print test summary
    print_summary
}
main
