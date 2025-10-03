#!/bin/bash

## Uncomment the following lines to enable debug mode
set -x

exec 2> k3kcli_install.log

version=${1}
platform=${2}
arch=$(uname -m)

download_retry() {
  cmd=${1}
  max_attempts=3
  attempt_num=1

  while [[ $attempt_num -le $max_attempts ]]; do
    if eval "$cmd"; then
      echo "Command succeeded after $attempt_num attempts."
      break
    else
      echo "Attempt $attempt_num failed. Retrying in 5 seconds..."
      attempt_num=$((attempt_num + 1))
      sleep 5
    fi
  done

  if [[ $attempt_num -ge $max_attempts ]]; then
    echo "Command failed after $max_attempts attempts."
  fi
}

install_k3kcli() {
    if [ "$arch" = "aarch64" ]; then
        KUBE_ARCH="arm64"
    else
        KUBE_ARCH="amd64"
    fi
    echo -e "Installing k3kcli version: $version..."
    download_retry "wget https://github.com/rancher/k3k/releases/download/$version/k3kcli-$platform-$KUBE_ARCH"
    sleep 10
    sudo cp k3kcli-$platform-$KUBE_ARCH /usr/local/bin/k3kcli && \
    chmod +x /usr/local/bin/k3kcli && \
    k3kcli --version
}

install_helm() {
    # TODO: Add check if helm is installed
    echo -e "Installing helm..."
    download_retry "curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3"
    sleep 10
    chmod +x ./get_helm.sh && \
    ./get_helm.sh
}

add_repo() {
    # TODO: Add check if repo is added
    echo -e "Adding k3k to helm repo..."
    helm repo add k3k https://rancher.github.io/k3k && \
    helm repo update
}

set_fs_inotify() {
    echo -e "Setting fs options using sysctl"
    sysctl -w fs.inotify.max_user_instances=2099999999 && \
    sysctl -w fs.inotify.max_queued_events=2099999999 && \
    sysctl -w fs.inotify.max_user_watches=2099999999
}

create_ns() {
    echo -e "Installing k3k using helm..."
    helm install --namespace k3k-system --create-namespace k3k k3k/k3k --devel
}

set_path() {
    echo -e "Setting kubconfig and PATH..."
    export KUBECONFIG=/etc/rancher/rke2/rke2.yaml && \
    export PATH=$PATH:/var/lib/rancher/rke2/bin:/opt/bin:/usr/local/bin
}

apply_lps() {
    echo -e "Applying local path storage and patching storageclass via annonation..."
    kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.30/deploy/local-path-storage.yaml && \
    kubectl patch storageclass local-path -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
}

main() {
    set_path
    apply_lps
    install_helm
    add_repo
    install_k3kcli
    set_fs_inotify
    create_ns
}

main "$@"
