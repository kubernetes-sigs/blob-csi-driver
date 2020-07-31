# Install blobfuse CSI driver v0.6.0 version on a kubernetes cluster

If you have already installed Helm, you can also use it to install blobfuse CSI driver. Please see [Installation with Helm](../charts/README.md).

## Install with kubectl
 - remote install
```console
curl -skSL https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/v0.6.0/deploy/install-driver.sh | bash -s v0.6.0 --
```

 - local install
```console
git clone https://github.com/kubernetes-sigs/blobfuse-csi-driver.git
cd blobfuse-csi-driver
./deploy/install-driver.sh v0.6.0 local
```

- check pods status:
```console
kubectl -n kube-system get pod -o wide -l app=csi-blobfuse-controller
kubectl -n kube-system get pod -o wide -l app=csi-blobfuse-node
```

example output:

```console
NAME                                           READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blobfuse-controller-56bfddd689-dh5tk       6/6     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-blobfuse-controller-56bfddd689-8pgr4       6/6     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-blobfuse-node-cvgbs                        3/3     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-blobfuse-node-dr4s4                        3/3     Running   0          35s     10.240.0.4     k8s-agentpool-22533604-0
```

- clean up blobfuse CSI driver
```console
curl -skSL https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/v0.6.0/deploy/uninstall-driver.sh | bash -s v0.6.0 --
```
