#!/bin/bash
# script to wrap docker run commands
PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

source ./config/.env

# Refuse to launch if AWS credentials weren't exported.
if [ -z "${AWS_ACCESS_KEY_ID:-}" ] || [ -z "${AWS_SECRET_ACCESS_KEY:-}" ]; then
    echo "ERROR: AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY must be exported in the shell" >&2
    echo "  Local: export AWS_ACCESS_KEY_ID=\$(aws configure get aws_access_key_id)" >&2
    echo "         export AWS_SECRET_ACCESS_KEY=\$(aws configure get aws_secret_access_key)" >&2
    echo "  CI:    inject before invoking this script (Jenkins withCredentials, GH secrets)" >&2
    exit 1
fi

TAG_NAME=${TAG_NAME:=distros-test}
CONTAINER_ROOT="/go/src/github.com/rancher/distros-test-framework"
KEY_CONTAINER_PATH="${CONTAINER_ROOT}/config/.ssh/aws_key.pem"
RUNTIME_INPUT_DIR="/run/dtf"

# Write AWS credentials to a temp env file instead of passing them on the CLI.
AWS_ENV_FILE=$(mktemp /tmp/aws-env-XXXXXX)
chmod 600 "$AWS_ENV_FILE"
trap 'rm -f "$AWS_ENV_FILE"' EXIT
cat > "$AWS_ENV_FILE" <<EOF
AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID}
AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY}
EOF

DOCKER_RUNTIME_ARGS=(
  --env-file "$AWS_ENV_FILE"
  --env-file ./config/.env
  -e "SSH_USER=${SSH_USER}"
  --mount "type=bind,source=${PWD}/config/.env,target=${RUNTIME_INPUT_DIR}/config.env,readonly"
  -e "DTF_CONFIG_ENV_SOURCE=${RUNTIME_INPUT_DIR}/config.env"
  --mount "type=bind,source=${SSH_LOCAL_KEY_PATH},target=${KEY_CONTAINER_PATH},readonly"
)

if [[ "${PROVISIONER_MODULE:-legacy}" == "qainfra" ]]; then
  QA_INFRA_TFVARS_PATH="${PWD}/infrastructure/qainfra/vars.tfvars"
  DOCKER_RUNTIME_ARGS+=(
    --mount "type=bind,source=${QA_INFRA_TFVARS_PATH},target=${RUNTIME_INPUT_DIR}/qainfra.tfvars,readonly"
    -e "DTF_QA_INFRA_TFVARS_SOURCE=${RUNTIME_INPUT_DIR}/qainfra.tfvars"
  )
else
  LEGACY_TFVARS_PATH="${PWD}/config/${ENV_PRODUCT}.tfvars"
  DOCKER_RUNTIME_ARGS+=(
    --mount "type=bind,source=${LEGACY_TFVARS_PATH},target=${RUNTIME_INPUT_DIR}/legacy.tfvars,readonly"
    -e "DTF_LEGACY_TFVARS_SOURCE=${RUNTIME_INPUT_DIR}/legacy.tfvars"
  )
fi

# Runs a Docker container with the specified image name and tag read from .env file.
test_run() {
  echo -e "\nRunning docker run script with:\ncontainer name: ${IMG_NAME}\ntag: ${TAG_NAME}\nproduct: ${ENV_PRODUCT}\n\n"
  run=$(docker run -dt --name "acceptance-test-${IMG_NAME}" \
    "${DOCKER_RUNTIME_ARGS[@]}" \
    -v "${HOME}/.aws:/root/.aws:ro" \
    "acceptance-test-${TAG_NAME}")

    if ! [ "$run" ]; then
      echo "Failed to run acceptance-test-${IMG_NAME} container."
      exit 1
    else
      echo -e "\nContainer started successfully."
      image_stats "${IMG_NAME}"
      docker logs -f "acceptance-test-${IMG_NAME}"
    fi
}

# Runs a new Docker container with a random suffix and version-specific image name
# Uses the same base image as the original container, so dont need to rebuild.
test_run_new() {
    RANDOM_SUFFIX=$(LC_ALL=C tr -dc 'a-z' </dev/urandom | head -c3)

    NEW_IMG_NAME=""
    if [[ -n "${RKE2_VERSION}" ]]; then
        NEW_IMG_NAME=$(echo "${RKE2_VERSION}" | sed 's/+.*//')
    elif [[ -n "${K3S_VERSION}" ]]; then
        NEW_IMG_NAME=$(echo "${K3S_VERSION}" | sed 's/+.*//')
    fi

    FULL_IMG_NAME="${IMG_NAME}-${NEW_IMG_NAME}-${RANDOM_SUFFIX}"
    echo -e "\nRunning docker run script with:\ncontainer name: ${FULL_IMG_NAME}\ntag: ${TAG_NAME}\nproduct: ${ENV_PRODUCT}\n\n"
    run=$(docker run -dt --name "acceptance-test-${FULL_IMG_NAME}" \
      "${DOCKER_RUNTIME_ARGS[@]}" \
      -v "${HOME}/.aws:/root/.aws:ro" \
      "acceptance-test-${TAG_NAME}")

      if ! [ "$run" ]; then
        echo "Failed to run acceptance-test-${IMG_NAME} container."
        exit 1
      else
        echo -e "\nContainer started successfully."
        image_stats "${FULL_IMG_NAME}"
        docker logs -f "acceptance-test-${FULL_IMG_NAME}"
      fi
}

# Container snapshots preserve runtime environment variables and copied secrets.
# Reuse infrastructure through test-run-updates instead.
test_run_state() {
    echo "test-run-state is disabled because image snapshots persist runtime secrets." >&2
    echo "Use test-run-updates to reuse infrastructure without creating an image." >&2
    exit 1
}

# Updates tracked source in a stopped test container and reruns its original command.
# State and runtime secrets remain scoped to that container until test-env-down.
test_run_updates() {
    CONTAINER_ID=$(docker ps -a -q --filter "ancestor=acceptance-test-${TAG_NAME}" | head -n 1)

    if [ -z "${CONTAINER_ID}" ]; then
        echo "No matching container found."
        exit 1
    fi
    if [ "$(docker inspect -f '{{.State.Running}}' "${CONTAINER_ID}")" = "true" ]; then
        echo "Matching container is still running: ${CONTAINER_ID}" >&2
        exit 1
    fi
    if ! git diff --quiet --diff-filter=D HEAD --; then
        echo "Source deletions require a fresh image and container." >&2
        exit 1
    fi

    update_dir=$(mktemp -d "${TMPDIR:-/tmp}/dtf-source-update.XXXXXX")
    mkdir "$update_dir/source"
    if ! git ls-files -z | tar --null -T - -cf "$update_dir/source.tar"; then
        rm -rf -- "$update_dir"
        echo "Failed to stage tracked source files." >&2
        exit 1
    fi
    if ! tar -xf "$update_dir/source.tar" -C "$update_dir/source"; then
        rm -rf -- "$update_dir"
        echo "Failed to extract tracked source files." >&2
        exit 1
    fi
    if ! docker cp "$update_dir/source/." "${CONTAINER_ID}:${CONTAINER_ROOT}"; then
        rm -rf -- "$update_dir"
        echo "Failed to update tracked source in the container." >&2
        exit 1
    fi
    rm -rf -- "$update_dir"

    echo -e "\nRestarting container with updated tracked source."
    docker start -a "${CONTAINER_ID}"
}

# Collects and logs Docker container stats.
image_stats() {
    local container_name=$1

    if [ -n "${container_name}" ]; then
      ./scripts/docker_stats.sh "${container_name}" 2>> /tmp/image-"${container_name}"_stats_output.log &
    else
      echo "No container name provided."
    fi
}

# Displays logs of the running Docker container
test_logs() {
   docker logs -f "acceptance-test-${IMG_NAME}"
}

# Builds the Docker image for the test environment
test_env_up() {
    docker build . -q -f ./scripts/Dockerfile.build -t acceptance-test-"${TAG_NAME}"
}

# Cleans up the test environment by removing containers,images and dangling images.
clean_env() {
  read -p "Remove local containers and images? [y/n]: " -n 1 -r
  if [[ $REPLY =~ ^[Yyes]$ ]]; then
    echo -e "\nRemoving acceptance-test containers"
    docker ps -a -q --filter="name=acceptance-test*" | xargs -r docker rm -f 2>/tmp/container_"${IMG_NAME}".log || true

    echo "Removing acceptance-test images"
    docker images -q --filter="reference=acceptance-test*" | xargs -r docker rmi -f 2>/tmp/container_"${IMG_NAME}".log || true

    echo "Removing dangling images"
    docker images -q -f "dangling=true" | xargs -r docker rmi -f 2>/tmp/container_"${IMG_NAME}".log || true

    echo "Removing state images"
    docker images -q --filter="reference=teststate:latest" | xargs -r docker rmi -f 2>/tmp/container_"${IMG_NAME}".log || true
    else
      echo "Exiting without removing containers and images."
  fi
}

case "$1" in
    test-build-run)
        test_env_up
        test_run_new
        ;;
    test-env-up)
        test_env_up
        ;;
    test-env-down)
        clean_env
        ;;
    test-run)
        test_run
        ;;
    test-run-new)
        test_run_new
      	;;
    test-run-state)
        test_run_state
       ;;
    test-run-updates)
         test_run_updates
        ;;
    image-stats)
         image_stats
        ;;
    test-logs)
         test_logs
        ;;
    *)
        echo "Unsupported command."
        exit 1
        ;;
esac
