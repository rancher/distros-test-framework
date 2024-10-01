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
    docker run -v "$PWD"/certs:/certs -e CA_SUBJECT="My own root CA" -e CA_EXPIRE="1825" -e SSL_EXPIRE="365" -e SSL_SUBJECT="$hostname" -e SSL_DNS="$hostname" -e SILENT="true" superseb/omgwtfssl
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
    curl -L "https://github.com/docker/compose/releases/download/v2.28.0/docker-compose-$os-$arch" -o /usr/local/bin/docker-compose && \
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
