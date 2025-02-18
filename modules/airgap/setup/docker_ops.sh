#!/bin/bash


## Uncomment the following lines to enable debug mode
# set -x
# echo "$@"

# Usage: ./docker_ops product hostdns username password registry_url
# Usage: ./docker_ops k3s ec2-host.com testuser testpass example.registry.com

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
  while read -r line; do
    if [[ -n "$registry_url" ]]; then
      line="${line/docker.io/$registry_url}"
    fi
    echo "Pulling image: $line"
    docker pull "$line"
    img="$line"
    if [[ "$line" =~ "docker" ]]; then
      img="${img/docker.io\/}"
    fi
    if [[ -n "$registry_url" ]] && [[ "$line" =~ $registry_url ]]; then
      img="${img/$registry_url\/}"
    fi
    echo "Tagging image: $img"
    docker image tag "$line" "$hostdns"/"$img"
    echo "Pushing image: $img"
    echo "$password" | docker login "$hostdns" -u "$username" --password-stdin && \
    docker push "$hostdns"/"$img"
    echo "Docker pull/tag/push completed for image: $img"
  done < "$image_file"
done

