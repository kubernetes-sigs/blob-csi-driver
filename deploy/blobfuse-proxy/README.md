# Blobfuse Proxy
 - supported CSI driver version: v1.1.0 or later version
 - only available on debian based agent node

By default, restart csi-blobfuse-node daemonset would make current blobfuse mounts unavailable. When fuse nodeserver restarts on the node, the fuse daemon also restarts, this results in breaking all connections FUSE daemon is maintaining. You could find more details here: [No easy way how to update CSI driver that uses fuse](https://github.com/kubernetes/kubernetes/issues/70013).

This guide shows how to install a blobfuse proxy on all agent nodes and the proxy would mount volumes, maintain FUSE connections.

### Install Blob CSI driver with `node.enableBlobfuseProxy=true`
 - helm install
```console
helm repo add blob-csi-driver https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/charts
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system --version v1.6.0 --set node.enableBlobfuseProxy=true
```

### Enable blobfuse proxy on existing Blob CSI driver
 - install blobfuse proxy
> following config only works on debian based agent node
```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/blobfuse-proxy.yaml
```
 - set `enable-blobfuse-proxy=true` in existing `csi-blob-node` daemonset (default is `false`)
