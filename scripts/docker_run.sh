#!/bin/bash
# script to wrap docker run commands
PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

source ./config/.env

# Derive mount target filename from ACCESS_KEY_LOCAL so we don't hardcode key names
KEY_BASENAME="$(basename "${ACCESS_KEY_LOCAL}")"
KEY_CONTAINER_PATH="/go/src/github.com/rancher/distros-test-framework/config/.ssh/${KEY_BASENAME}"
PUB_KEY_CONTAINER_PATH="${KEY_CONTAINER_PATH}.pub"

if [ -z "${TAG_NAME}" ]; then
    TAG_NAME="distros"
fi

# Runs a Docker container with the specified image name and tag read from .env file.
test_run() {
  echo -e "\nRunning docker run script with:\ncontainer name: ${IMG_NAME}\ntag: ${TAG_NAME}\nproduct: ${ENV_PRODUCT}\n\n"
  run=$(docker run -dt --name "acceptance-test-${IMG_NAME}" \
    -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
    -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
    -e ACCESS_KEY_FILE="${KEY_CONTAINER_PATH}" \
    --env-file ./config/.env \
    -v "${ACCESS_KEY_LOCAL}:${KEY_CONTAINER_PATH}:ro" \
    -v "${ACCESS_KEY_LOCAL}.pub:${KEY_CONTAINER_PATH}.pub:ro" \
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
      -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
      -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
      -e ACCESS_KEY_FILE="${KEY_CONTAINER_PATH}" \
      --env-file ./config/.env \
      -v "${ACCESS_KEY_LOCAL}:${KEY_CONTAINER_PATH}:ro" \
    -v "${ACCESS_KEY_LOCAL}.pub:${KEY_CONTAINER_PATH}.pub:ro" \
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

# Commits the state of a previous running container and runs a new container from that state
# This is useful for running other tests with the same state as a previous test, meaning using the same terraform state
# using same instances.
test_run_state() {
     CONTAINER_ID=$(docker ps -a -q --filter "ancestor=acceptance-test-${TAG_NAME}" | head -n 1)

     if [ -z "${CONTAINER_ID}" ]; then
         echo "No matching container found."
         exit 1
     fi

     if docker commit "${CONTAINER_ID}" teststate:latest; then
         if docker run -dt --name "acceptance-test-${TEST_STATE}" --env-file ./config/.env \
             -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
             -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
             -e ACCESS_KEY_FILE="${KEY_CONTAINER_PATH}" \
             -v "${ACCESS_KEY_LOCAL}:${KEY_CONTAINER_PATH}:ro" \
    -v "${ACCESS_KEY_LOCAL}.pub:${KEY_CONTAINER_PATH}.pub:ro" \
             -v "$(pwd)/scripts/test-runner.sh:/go/src/github.com/rancher/distros-test-framework/scripts/test-runner.sh" \
             teststate:latest; then
             echo "\nRunning docker run script with:\ncontainer name: ${IMG_NAME}\ntag: ${TAG_STATE}\product: ${ENV_PRODUCT}\n\n"
             docker logs -f "acceptance-test-${TEST_STATE}"
         else
             echo "Failed to start the container from the committed state."
             exit 1
         fi
     else
         echo "Failed to commit container."
         exit 1
     fi
}

# Copies /tmp files for config and terraform modules from a previous running container,
# and starts a new container with the same state as the previous container.
# This is useful for running other tests and also you can change the current code and run the tests again within the same instances.
test_run_updates() {
    CONTAINER_ID=$(docker ps -a -q --filter "ancestor=acceptance-test-${TAG_NAME}" | head -n 1)

    RANDOM_SUFFIX=$(LC_ALL=C tr -dc 'a-z' </dev/urandom | head -c3)
    NEW_IMG_NAME="${IMG_NAME}-${NEW_IMG_NAME}-${RANDOM_SUFFIX}"

    if [ -z "${CONTAINER_ID}" ]; then
        echo "No matching container found."
        exit 1
    else
        docker cp "${CONTAINER_ID}:/tmp/" tmp/
        docker cp "${CONTAINER_ID}:/go/src/github.com/rancher/distros-test-framework/infrastructure/" tmp/infrastructure/

        test_env_up "${TAG_NAME}"
        
        run=$(docker run -dt --name "acceptance-test-${NEW_IMG_NAME}" \
            -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
            -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
            -e ACCESS_KEY_FILE="${KEY_CONTAINER_PATH}" \
            --env-file ./config/.env \
            -v "${ACCESS_KEY_LOCAL}:${KEY_CONTAINER_PATH}:ro" \
    -v "${ACCESS_KEY_LOCAL}.pub:${KEY_CONTAINER_PATH}.pub:ro" \
            -v "${PWD}/scripts/test-runner.sh:/go/src/github.com/rancher/distros-test-framework/scripts/test-runner.sh" \
            -v "${PWD}/tmp/infrastructure/:/go/src/github.com/rancher/distros-test-framework/infrastructure/" \
            -v "${PWD}/tmp/:/tmp" \
            "acceptance-test-${TAG_NAME}")

      if ! [ "$run" ]; then
        echo "Failed to run updated acceptance-test-${NEW_IMG_NAME} container."
        exit 1
      else
        echo -e "\nContainer started successfully."
        image_stats "${NEW_IMG_NAME}"
        docker logs -f "acceptance-test-${NEW_IMG_NAME}"
      fi
    fi
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
