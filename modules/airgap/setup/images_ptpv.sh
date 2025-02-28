#!/bin/bash


## Uncomment the following lines to enable debug mode
# set -x
# echo "$@"

# Perform image pull/tag/push/validate operations on listed images
# Usage: ./images_ptpv product hostdns username password registry_url
# Usage: ./images_ptpv k3s ec2-host.com testuser testpass example.registry.com

# Define the input file
product=${1}
hostdns=${2}
username=${3}
password=${4}
registry_url=${5}

IFS=$'\n' # set the Internal Field Separator to newline

image_files=$(ls "$product"*.txt)
image_files=$(echo "$image_files" | tr " " "\n")
echo "$image_files"
for image_file in $image_files; do
  echo "Reading from file: $image_file"
  while read -r line; do
    if [[ -n "$registry_url" ]]; then
      line="${line/docker.io/$registry_url}"
    fi
    echo "Pulling image: $line"
    if [[ "$line" =~ "windows" ]]; then
      podman pull "$line" --platform windows/amd64
    else
      podman pull "$line"
    fi
    img="$line"
    if [[ "$line" =~ "docker" ]]; then
      img="${img/docker.io\/}"
    fi
    if [[ -n "$registry_url" ]] && [[ "$line" =~ $registry_url ]]; then
      img="${img/$registry_url\/}"
    fi
    echo "Tagging image: $img"
    podman image tag "$line" "$hostdns"/"$img"
    echo "Pushing image: $img"
    echo "$password" | podman login "$hostdns" -u "$username" --password-stdin && \
    podman push "$hostdns"/"$img"
    echo "Pull/Tag/Push completed for image: $img"
    echo "Inspecting image: $img"
    podman image inspect "$hostdns"/"$img"
  done < "$image_file"
done

