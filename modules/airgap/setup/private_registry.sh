#!/bin/bash

# Usage: ./private_registry.sh username password hostdns

## Uncomment the following lines to enable debug mode
# set -x
# echo "$@"

username=${1}
password=${2}
hostname=${3}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)

if [ -z "$hostname" ]; then
  hostname=$(hostname -f)
fi

generate_certs() {
    docker run -v "$PWD"/certs:/certs \
    -e CA_SUBJECT="My own root CA" \
    -e CA_EXPIRE="1825" \
    -e SSL_EXPIRE="365" \
    -e SSL_SUBJECT="$hostname" \
    -e SSL_DNS="$hostname" \
    -e SILENT="true" superseb/omgwtfssl
}

move_certs() {
    cat certs/cert.pem certs/ca.pem > basic-registry/nginx_config/domain.crt
    cat certs/key.pem > basic-registry/nginx_config/domain.key
}

save_creds() {
    docker run --rm melsayed/htpasswd "$username" "$password" >> basic-registry/nginx_config/registry.password
    mkdir -p /etc/docker/certs.d/"$hostname"
    cp certs/ca.pem /etc/docker/certs.d/"$hostname"/ca.crt
    service docker restart
}

docker_compose() {
    cd basic-registry && \
    COMPOSE_VERSION="v2.28.0" && \
    if [ "$arch" = "aarch64" ]; then \
        COMPOSE_SHA256="296076f4d14d2a816ad750f6890355fc118692814e4b4542942794817f869d37"; \
    else \
        COMPOSE_SHA256="359043c2336e243662d7038c3edfeadcd5b9fc28dabe6973dbaecf48c0c1f967"; \
    fi && \
    curl -L "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-$os-$arch" -o /usr/local/bin/docker-compose && \
    echo "${COMPOSE_SHA256}  /usr/local/bin/docker-compose" | sha256sum -c - && \
    chmod +x /usr/local/bin/docker-compose && \
    /usr/local/bin/docker-compose up -d && \
    cd ..
}

main() {
    service docker restart
    generate_certs
    move_certs
    save_creds
    docker_compose
}
main "$@"
