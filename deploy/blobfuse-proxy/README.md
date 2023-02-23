# Blobfuse Proxy
 - supported CSI driver version: v1.6.0+
 - only available on **debian** OS based agent node (not available on OpenShift)

By default, restart csi-blobfuse-node daemonset would make current blobfuse mounts unavailable. When fuse nodeserver restarts on the node, the fuse daemon also restarts, this results in breaking all connections FUSE daemon is maintaining. You could find more details here: [No easy way how to update CSI driver that uses fuse](https://github.com/kubernetes/kubernetes/issues/70013).

This guide shows how to install a blobfuse proxy on all agent nodes and the proxy would mount volumes, maintain FUSE connections.

### Step#2. Install Blob CSI driver with blobfuse-proxy enabled
 - helm install
```console
helm repo add blob-csi-driver https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/charts
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system --version v1.20.0 --set node.enableBlobfuseProxy=true
```

 - kubectl install
```console
curl -skSL https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/v1.20.0/deploy/install-driver.sh | bash -s v1.20.0 blobfuse-proxy --
```

### Enable blobfuse proxy on existing Blob CSI driver
 - install blobfuse proxy daemonset
> following config only works on debian based agent node
```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/v1.20.0/blobfuse-proxy.yaml
```
 - set `enable-blobfuse-proxy=true` in existing `csi-blob-node` daemonset manually (default is `false`)
```console
kubectl edit ds csi-blob-node -n kube-system
```
