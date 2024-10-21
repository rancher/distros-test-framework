#!/bin/bash

# set -x
# echo "$@"

hostname=${1}

if [ -z "$hostname" ]; then
  hostname=$(hostname -f)
fi

generate_certs() {
    mkdir -p certs
    openssl req -newkey rsa:4096 -nodes -sha256 \
    -keyout certs/domain.key -x509 -days 365 -out certs/domain.crt \
    -subj "/C=US/ST=AZ/O=Rancher QA/CN=$hostname" -addext "subjectAltName = DNS:$hostname"
}

update_docker() {
    mkdir -p /etc/docker/certs.d/"$hostname" && \
    cp certs/domain.crt /etc/docker/certs.d/"$hostname"/ca.crt && \
    service docker restart
}

run_private_registry() {
    docker run -d --restart=always --name registry \
    -v "$PWD"/certs:/certs -e REGISTRY_HTTP_ADDR=0.0.0.0:443 \
    -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt \
    -e REGISTRY_HTTP_TLS_KEY=/certs/domain.key -p 443:443 registry:2
}

main() {
    generate_certs
    update_docker
    run_private_registry
}
main "$@"