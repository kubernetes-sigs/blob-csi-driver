# Install Azure Blob Storage CSI driver v1.0.0 version on a kubernetes cluster

If you have already installed Helm, you can also use it to install Azure Blob Storage CSI driver. Please see [Installation with Helm](../charts/README.md).

## Install with kubectl
 - remote install
```console
curl -skSL https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/v1.0.0/deploy/install-driver.sh | bash -s v1.0.0 --
```

 - local install
```console
git clone https://github.com/kubernetes-sigs/blob-csi-driver.git
cd blob-csi-driver
./deploy/install-driver.sh v1.0.0 local
```

- check pods status:
```console
kubectl -n kube-system get pod -o wide -l app=csi-blob-controller
kubectl -n kube-system get pod -o wide -l app=csi-blob-node
```

example output:

```console
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blob-controller-56bfddd689-dh5tk       4/4     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-blob-controller-56bfddd689-8pgr4       4/4     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-blob-node-cvgbs                        3/3     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-blob-node-dr4s4                        3/3     Running   0          35s     10.240.0.4     k8s-agentpool-22533604-0
```

- clean up Azure Blob Storage CSI driver
```console
curl -skSL https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/v1.0.0/deploy/uninstall-driver.sh | bash -s v1.0.0 --
```
