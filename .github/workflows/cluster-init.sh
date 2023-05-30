#!/bin/bash

set -ex

max=10

retry() {
    i=1
    echo "Attempt $i"
    while ! $*; do
        sleep 3
        i=$(($i + 1))
        if [ $i -gt $max ]; then
            echo "ERROR: too many retries"
            exit 1
        fi
        echo "Attempt $i"
    done
}

curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-archive-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-archive-keyring.gpg] https://apt.kubernetes.io/ kubernetes-xenial main" | sudo tee /etc/apt/sources.list.d/kubernetes.list
sudo apt-get update
sudo apt-get install -y kubelet kubeadm kubectl
sudo swapoff -a
sudo kubeadm init -v 5 ${CRI_SOCKET_INIT_ARG} --pod-network-cidr=192.168.0.0/16 || (sudo journalctl -u kubelet; exit 1)
mkdir -p $HOME/.kube/
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $USER $HOME/.kube/config
echo "Waiting for k8s to bootstrap"
sleep 20
retry curl -k https://localhost:6443/
sleep 5
retry kubectl taint nodes --all node-role.kubernetes.io/control-plane-
kubectl create -f https://raw.githubusercontent.com/projectcalico/calico/v3.25.0/manifests/tigera-operator.yaml
sleep 2
kubectl create -f https://raw.githubusercontent.com/projectcalico/calico/v3.25.0/manifests/custom-resources.yaml
sleep 2
kubectl wait --for=condition=ready --timeout=30s node --all
kubectl get nodes -o wide
kubectl get all --all-namespaces
kubectl wait --for=condition=ready --timeout=300s --all-namespaces pod --all
# kubectl wait --for=condition=ready --timeout=300s --all-namespaces deployment --all
kubectl get all --all-namespaces
sleep 2
