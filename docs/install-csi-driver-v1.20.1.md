# Install Azure Blob Storage CSI driver v1.20.1 version on a kubernetes cluster
> `blobfuse-proxy` is only available for debian based agent nodes, remove `blobfuse-proxy` parameter in installation steps if it's not applicable.
> 
If you have already installed Helm, you can also use it to install this driver. Please check [Installation with Helm](../charts/README.md).

## Install with kubectl
 - Option#1. remote install
```console
curl -skSL https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/v1.20.1/deploy/install-driver.sh | bash -s v1.20.1 blobfuse-proxy --
```

 - Option#2. local install
```console
git clone https://github.com/kubernetes-sigs/blob-csi-driver.git
cd blob-csi-driver
./deploy/install-driver.sh v1.20.1 local,blobfuse-proxy
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

### clean up Blob CSI driver
- Option#1. remote uninstall
```console
curl -skSL https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/v1.20.1/deploy/uninstall-driver.sh | bash -s v1.20.1 --
```

 - Option#2. local uninstall
```console
git clone https://github.com/kubernetes-sigs/blob-csi-driver.git
cd blob-csi-driver
git checkout v1.20.1
./deploy/uninstall-driver.sh v1.20.1 local
```
