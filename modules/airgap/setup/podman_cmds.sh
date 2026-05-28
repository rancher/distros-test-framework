#!/bin/bash
set -e
exec > >(tee -a podman_cmds.log) 2>&1

# Usage: ./podman_cmds.sh <product> <platform> <hostdns> <username> <password> [registry_url]

product=${1}
platform=${2}
hostdns=${3}
username=${4}
password=${5}
registry_url=${6} 

echo "Logging into private registry: $hostdns..."
echo "$password" | podman login "$hostdns" -u "$username" --password-stdin

shopt -s nullglob
if [[ "$product" == "k3s" ]]; then
  image_files=( "$product"*.txt )
else
  image_files=( *"$platform"*.txt )
fi
shopt -u nullglob

if [[ ${#image_files[@]} -eq 0 ]]; then
  echo "No matching text files found."
  exit 0
fi

echo "Found image files: ${image_files[*]}"

for image_file in "${image_files[@]}"; do
  echo "Reading from file: $image_file"
  
  while read -r image_url_tag || [[ -n "$image_url_tag" ]]; do
    [[ -z "$image_url_tag" ]] && continue

    echo "Pulling image: $image_url_tag"
    if [[ "$image_url_tag" == *"windows"* ]]; then
      podman pull "$image_url_tag" --platform windows/amd64
    else
      podman pull "$image_url_tag"
    fi

    # 1. Isolate the first segment (everything before the first '/')
    first_segment="${image_url_tag%%/*}"

    # 2. Check if the first segment looks like a registry domain
    if [[ "$first_segment" == *.* ]] || [[ "$first_segment" == *:* ]] || [[ "$first_segment" == "docker.io" ]]; then
      # It IS a registry. Strip the registry and the first slash.
      img="${image_url_tag#*/}"
    else
      # It is NOT a registry (e.g., "rancher/k3s:v1.28"). Keep it as is.
      img="$image_url_tag"
    fi

    target_image="$hostdns/$img"

    echo "Tagging image as: $target_image"
    podman image tag "$image_url_tag" "$target_image"
    
    echo "Pushing image: $target_image"
    podman push "$target_image"
    
    echo "Validating image: $target_image"
    if podman inspect "$target_image" > /dev/null 2>&1; then
      echo "Validation successful! Image is fully intact."
    else
      echo "ERROR: Validation failed! Image $target_image is corrupted or missing." >&2
      exit 1
    fi
    
    echo "Successfully migrated: $img"
    echo "----------------------------------------"
  done < "$image_file"
done

echo "All images processed successfully!"