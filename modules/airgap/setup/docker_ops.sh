#!/bin/bash

set -x
echo "$@"

# Define the input file
product=${1}
hostdns=${2}
username=${3}
password=${4}

IFS=$'\n' # set the Internal Field Separator to newline

image_files=`ls $product*.txt`
image_files=$(echo $image_files | tr " " "\n")
echo $image_files
for image_file in $image_files; do
  for line in $(cat "$image_file"); do
    if [[ "$line" =~ "docker" ]]; then
      line=`echo "${line/docker.io\/}"`
    fi
    docker pull $line
    docker image tag $line $hostdns/$line
    echo "$password" | docker login $hostdns -u "$username" --password-stdin && \
    docker push $hostdns/$line
    echo "Docker pull/tag/push completed for image: $line"
  done
done

