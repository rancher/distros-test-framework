#!/bin/bash

## Uncomment the following lines to enable debug mode
# set -x
# echo "$@"

# Perform image pull/tag/push/validate operations on listed images
# Usage: ./podman_cmds product hostdns username password registry_url
# Usage: ./podman_cmds k3s ec2-host.com testuser testpass example.registry.com

# Define the input file
product=${1}
platform=${2}
hostdns=${3}
username=${4}
password=${5}
registry_url=${6}

IFS=$'\n' # set the Internal Field Separator to newline

if [[ "$product" == "k3s" ]]; then
  image_files=$(ls "$product"*.txt)
else
  image_files=$(ls ./*"$platform"*.txt)
fi

image_files=$(echo "$image_files" | tr " " "\n")
echo "Found image files: $image_files"
for image_file in $image_files; do
  echo "Reading from file: $image_file"
  while read -r image_url_tag; do
    if [[ -n "$registry_url" ]]; then
      image_url_tag="${image_url_tag/docker.io/$registry_url}"
    fi
    echo "Pulling image: $image_url_tag"
    if [[ "$image_url_tag" =~ "windows" ]]; then
      podman pull "$image_url_tag" --platform windows/amd64
    else
      podman pull "$image_url_tag"
    fi
    img="$image_url_tag"
    if [[ "$image_url_tag" =~ "docker" ]]; then
      img="${img/docker.io\/}"
    fi
    if [[ -n "$registry_url" ]] && [[ "$image_url_tag" =~ $registry_url ]]; then
      img="${img/$registry_url\/}"
    fi
    echo "Tagging image: $img"
    podman image tag "$image_url_tag" "$hostdns"/"$img"
    echo "Pushing image: $img"
    echo "$password" | podman login "$hostdns" -u "$username" --password-stdin && \
    podman push "$hostdns"/"$img"
    echo "Pull/Tag/Push completed for image: $img"
  done < "$image_file"
done

