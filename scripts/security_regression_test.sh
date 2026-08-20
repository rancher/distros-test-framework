#!/bin/sh
set -eu

repo_root=$(unset CDPATH; cd -- "$(dirname "$0")/.." && pwd -P)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/dtf-security-test.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

container_root="$test_root/container"
mkdir -p "$container_root/config" "$container_root/infrastructure/qainfra" "$container_root/scripts" "$test_root/bin"
printf '%s\n' 'ENV_PRODUCT=k3s' 'PROVISIONER_MODULE=qainfra' 'DB_PASSWORD=DB_SENTINEL' > "$test_root/config.env"
printf '%s\n' \
    'aws_access_key = "AKIA_SENTINEL"' \
    'aws_secret_key = "SECRET_SENTINEL"' \
    'external_db_password = "DB_SENTINEL"' \
    'nodes = []' > "$test_root/qainfra.tfvars"
chmod 400 "$test_root/config.env" "$test_root/qainfra.tfvars"
qainfra_source_checksum=$(cksum "$test_root/qainfra.tfvars")
cat > "$container_root/scripts/test_runner.sh" <<'EOF'
#!/bin/sh
set -eu
test -f "$DTF_CONTAINER_ROOT/config/.env"
test -f "$DTF_CONTAINER_ROOT/infrastructure/qainfra/vars.tfvars"
test "$(grep -c '^nodes = ' "$DTF_CONTAINER_ROOT/infrastructure/qainfra/vars.tfvars")" -eq 1
grep -q 'count = 1' "$DTF_CONTAINER_ROOT/infrastructure/qainfra/vars.tfvars"
EOF
system_sed=$(command -v sed)
cat > "$test_root/bin/sed" <<'EOF'
#!/bin/sh
set -eu
: "${DTF_TEST_SYSTEM_SED:?}"
if [ "$1" = "-i" ]; then
    shift
    if [ "$(uname -s)" = "Darwin" ]; then
        exec "$DTF_TEST_SYSTEM_SED" -i '' "$@"
    fi
    exec "$DTF_TEST_SYSTEM_SED" -i "$@"
fi
exec "$DTF_TEST_SYSTEM_SED" "$@"
EOF
chmod 700 "$test_root/bin/sed"

output=$(
    cd "$container_root"
    DTF_TEST_SYSTEM_SED="$system_sed" PATH="$test_root/bin:$PATH" \
    DTF_CONTAINER_ROOT="$container_root" \
    DTF_CONFIG_ENV_SOURCE="$test_root/config.env" \
    DTF_QA_INFRA_TFVARS_SOURCE="$test_root/qainfra.tfvars" \
    PROVISIONER_MODULE=qainfra ENV_PRODUCT=k3s NO_OF_SERVER_NODES=1 \
    sh "$repo_root/scripts/entrypoint.sh" 2>&1
)
if printf '%s' "$output" | grep -Eq 'AKIA_SENTINEL|SECRET_SENTINEL|DB_SENTINEL'; then
    echo "entrypoint exposed a secret sentinel" >&2
    exit 1
fi
if [ "$(cksum "$test_root/qainfra.tfvars")" != "$qainfra_source_checksum" ]; then
    echo "entrypoint modified the read-only qainfra source" >&2
    exit 1
fi

dollar='$'
jenkinsfile="$repo_root/scripts/Jenkinsfile"
if grep -Eq '^[[:space:]]*ENTRYPOINT' "$repo_root/scripts/Dockerfile.jenkins" || \
    ! grep -Fq 'docker build . -f scripts/Dockerfile.jenkins' "$repo_root/scripts/build.sh"; then
    echo "Jenkins image execution contract changed; review its runtime mounts" >&2
    exit 1
fi
for required in \
    "target=\${containerRoot}/config/.env,readonly" \
    "target=\${containerRoot}/infrastructure/qainfra/vars.tfvars,readonly" \
    "'AWS_SSH_PEM_KEY='"; do
    if ! grep -Fq -- "$required" "$jenkinsfile"; then
        echo "Jenkins runtime contract is missing: $required" >&2
        exit 1
    fi
done
symlink_root="$test_root/symlink-container"
mkdir -p "$symlink_root/config" "$symlink_root/infrastructure/qainfra" "$symlink_root/scripts"
printf '%s\n' 'DO_NOT_OVERWRITE' > "$test_root/symlink-victim"
ln -s "$test_root/symlink-victim" "$symlink_root/config/.env"
cp "$container_root/scripts/test_runner.sh" "$symlink_root/scripts/test_runner.sh"
if ! symlink_output=$(
    cd "$symlink_root"
    DTF_TEST_SYSTEM_SED="$system_sed" PATH="$test_root/bin:$PATH" \
    DTF_CONTAINER_ROOT="$symlink_root" \
    DTF_CONFIG_ENV_SOURCE="$test_root/config.env" \
    DTF_QA_INFRA_TFVARS_SOURCE="$test_root/qainfra.tfvars" \
    PROVISIONER_MODULE=qainfra ENV_PRODUCT=k3s NO_OF_SERVER_NODES=1 \
    sh "$repo_root/scripts/entrypoint.sh" 2>&1
); then
    echo "entrypoint failed while replacing a destination symlink: $symlink_output" >&2
    exit 1
fi
if [ -L "$symlink_root/config/.env" ] || ! grep -q '^DO_NOT_OVERWRITE$' "$test_root/symlink-victim"; then
    echo "runtime staging followed an existing destination symlink" >&2
    exit 1
fi
if (
    cd "$container_root"
    DTF_CONTAINER_ROOT="$container_root" \
    DTF_CONFIG_ENV_SOURCE="$test_root/missing.env" \
    DTF_QA_INFRA_TFVARS_SOURCE="$test_root/qainfra.tfvars" \
    PROVISIONER_MODULE=qainfra ENV_PRODUCT=k3s NO_OF_SERVER_NODES=1 \
    sh "$repo_root/scripts/entrypoint.sh" >/dev/null 2>&1
); then
    echo "entrypoint accepted a missing runtime input" >&2
    exit 1
fi
if grep -Eq 'docker[[:space:]]+commit' "$repo_root/scripts/docker_run.sh"; then
    echo "docker commit can persist runtime credentials in an image" >&2
    exit 1
fi
if grep -Eq 'docker cp .*:\/?tmp|docker cp .*:/go/src/.*/infrastructure' \
    "$repo_root/scripts/docker_run.sh"; then
    echo "container state or secrets can be copied into the host workspace" >&2
    exit 1
fi

configure_root="$test_root/configure"
mkdir -p "$configure_root"
configure_output=$(
    cd "$configure_root"
    env DEBUG=true \
        AWS_ACCESS_KEY_ID=AKIA_CONFIG_SENTINEL \
        AWS_SECRET_ACCESS_KEY=SECRET_CONFIG_SENTINEL \
        AWS_SESSION_TOKEN=SESSION_CONFIG_SENTINEL \
        AWS_REGION=us-east-2 \
        AWS_SSH_PEM_KEY=PEM_CONFIG_SENTINEL \
        RKE2_TOKEN=RKE2_CONFIG_SENTINEL \
        bash "$repo_root/scripts/configure.sh" 2>&1
)
if printf '%s' "$configure_output" | grep -Eq 'AKIA_CONFIG_SENTINEL|SECRET_CONFIG_SENTINEL|SESSION_CONFIG_SENTINEL|PEM_CONFIG_SENTINEL|RKE2_CONFIG_SENTINEL'; then
    echo "configure.sh exposed a secret sentinel" >&2
    exit 1
fi
if ! grep -q '^AWS_REGION=us-east-2$' "$configure_root/.env" || \
    grep -Eq 'AWS_SSH_PEM_KEY|RKE2_TOKEN' "$configure_root/.env"; then
    echo "configure.sh did not enforce its environment allowlist" >&2
    exit 1
fi

for batch_file in Jenkinsfile_batch_install_test Jenkinsfile_batch_upgrade_test; do
    batch_path="$repo_root/scripts/$batch_file"
    if ! grep -Fq ".split(',', -1).collect { it.trim() }" "$batch_path" || \
        ! grep -Fq "error(\"Unknown qainfra batch test: ${dollar}{test_name}\")" "$batch_path"; then
        echo "$batch_file can silently ignore a requested test" >&2
        exit 1
    fi
done
for jenkins_path in "$repo_root"/scripts/Jenkinsfile*; do
    [ "$(basename "$jenkins_path")" = "Jenkinsfile_batch_airgap_test" ] && continue
    if grep -Fq "string(name: 'AWS_SSH_PEM_KEY'" "$jenkins_path" || \
        grep -Fq "addStrParam(param, 'AWS_SSH_PEM_KEY'" "$jenkins_path"; then
        echo "$(basename "$jenkins_path") can forward PEM material to child builds" >&2
        exit 1
    fi
done
for jenkins_file in Jenkinsfile Jenkinsfile_batch_os_validation Jenkinsfile_post_release_captain; do
    if ! grep -Fq "'AWS_SSH_PEM_KEY='" "$repo_root/scripts/$jenkins_file"; then
        echo "$jenkins_file does not clear inherited PEM material" >&2
        exit 1
    fi
done

update_root="$test_root/update-run"
mkdir -p "$update_root/config" "$update_root/infrastructure/qainfra" "$update_root/bin"
cp "$repo_root/scripts/docker_run.sh" "$update_root/docker_run.sh"
printf '%s\n' \
    'PROVISIONER_MODULE=qainfra' \
    'ENV_PRODUCT=k3s' \
    'SSH_USER=ec2-user' \
    "SSH_LOCAL_KEY_PATH=$update_root/key.pem" \
    'IMG_NAME=security-regression' > "$update_root/config/.env"
printf '%s\n' 'tracked content' > "$update_root/source.txt"
printf '%s\n' '*.tfvars' > "$update_root/.gitignore"
(
    cd "$update_root"
    git init -q
    git add .gitignore docker_run.sh source.txt
    git -c user.name=test -c user.email=test@example.invalid commit -qm baseline
)
printf '%s\n' 'updated tracked content' > "$update_root/source.txt"
printf '%s\n' 'HOST_SECRET_SENTINEL' > "$update_root/secret.tfvars"
cat > "$update_root/bin/docker" <<'EOF'
#!/bin/sh
set -eu
case "$1" in
    ps) printf '%s\n' test-container ;;
    inspect) printf '%s\n' false ;;
    cp)
        test -f "$2/source.txt"
        grep -q '^updated tracked content$' "$2/source.txt"
        test ! -e "$2/secret.tfvars"
        ;;
    start) test "$2" = '-a' && test "$3" = 'test-container' ;;
    *) echo "unexpected docker invocation: $*" >&2; exit 1 ;;
esac
EOF
chmod 700 "$update_root/bin/docker"
(
    cd "$update_root"
    PATH="$update_root/bin:$PATH" \
    AWS_ACCESS_KEY_ID=TEST_ACCESS AWS_SECRET_ACCESS_KEY=TEST_SECRET \
    bash ./docker_run.sh test-run-updates >/dev/null
)
log_pattern="TFVARS TEST DATA|println\\(testdata|cat[[:space:]]+\"${dollar}CONFIG_PATH\""
if grep -Eq "$log_pattern" \
    "$repo_root/scripts/Jenkinsfile" "$repo_root/scripts/entrypoint.sh"; then
    echo "full tfvars logging was reintroduced" >&2
    exit 1
fi
workspace="$test_root/workspace"
mkdir -p "$workspace/config/.ssh" "$workspace/infrastructure/qainfra"
for path in .env .qase.env config/.env config/k3s.tfvars config/rke2.tfvars \
    config/.ssh/aws_key.pem infrastructure/qainfra/vars.tfvars; do
    printf '%s\n' 'CLEANUP_SECRET_SENTINEL' > "$workspace/$path"
done
sh "$repo_root/scripts/cleanup_sensitive_files.sh" "$workspace"
sh "$repo_root/scripts/cleanup_sensitive_files.sh" "$workspace"
if find "$workspace" -type f -print -quit | grep -q .; then
    echo "sensitive files remained after cleanup" >&2
    exit 1
fi
