# Install blobfuse CSI driver development version on a kubernetes cluster

If you have already installed Helm, you can also use it to install blobfuse CSI driver. Please see [Installation with Helm](../charts/README.md).

## Installation with kubectl

```concole
kubectl apply -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/crd-csi-driver-registry.yaml
kubectl apply -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/crd-csi-node-info.yaml
kubectl apply -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/rbac-csi-blobfuse-controller.yaml
kubectl apply -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/csi-blobfuse-controller.yaml
kubectl apply -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/csi-blobfuse-node.yaml
```

- check pods status:

```concole
kubectl -n kube-system get pod -o wide -l app=csi-blobfuse-controller
kubectl -n kube-system get pod -o wide -l app=csi-blobfuse-node
```

example output:

```concole
NAME                                           READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blobfuse-controller-56bfddd689-dh5tk       6/6     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-blobfuse-controller-56bfddd689-8pgr4       6/6     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-blobfuse-controller-56bfddd689-ztp28       6/6     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-blobfuse-node-cvgbs                        3/3     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-blobfuse-node-dr4s4                        3/3     Running   0          35s     10.240.0.4     k8s-agentpool-22533604-0
```

- clean up blobfuse CSI driver

```concole
kubectl delete -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/csi-blobfuse-controller.yaml
kubectl delete -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/csi-blobfuse-node.yaml
kubectl delete -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/crd-csi-driver-registry.yaml
kubectl delete -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/crd-csi-node-info.yaml
kubectl delete -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/rbac-csi-blobfuse-controller.yaml
```
