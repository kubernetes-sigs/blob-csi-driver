# fsGroup Support on NFS protocol

[fsGroupPolicy](https://kubernetes-csi.github.io/docs/support-fsgroup.html) feature is supported from Kubernetes 1.20, follow below steps to enable this feature. Please note that blobfuse does not support fsGroupPolicy yet, only NFS protocol supports fsGroup.

### Option#1: Enable fsGroupPolicy support in [driver helm installation](../../../charts)

add `--set feature.enableFSGroupPolicy=true` in helm installation command.

### Option#2: Enable fsGroupPolicy support on a cluster with CSI driver already installed

```console
kubectl delete CSIDriver blob.csi.azure.com
cat <<EOF | kubectl create -f -
apiVersion: storage.k8s.io/v1
kind: CSIDriver
metadata:
  name: blob.csi.azure.com
spec:
  attachRequired: false
  podInfoOnMount: true
  fsGroupPolicy: File
  volumeLifecycleModes:
    - Persistent
    - Ephemeral
EOF
```
