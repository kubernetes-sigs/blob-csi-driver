#### Breaking change
From `v0.7.0`, driver name changed from `blobfuse.csi.azure.com` to `blob.csi.azure.com`, volume created by `v0.6.0`(or prior version) could not be mounted by `v0.7.0` driver. If you have volumes created by `v0.6.0` version, DO NOT upgrade to `v0.7.0` or higher version.

# Install CSI driver with Helm 3

## Prerequisites
 - [install Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

## install latest version
```console
helm repo add blob-csi-driver https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/charts
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system
```

## install on Azure Stack
```console
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system --set cloud=AzureStackCloud
```

### install a specific version
```console
helm repo add blob-csi-driver https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/charts
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system --version v0.10.0
```

### search for all available chart versions
```console
helm search repo -l blob-csi-driver
```

## uninstall CSI driver
```console
helm uninstall blob-csi-driver -n kube-system
```

## latest chart configuration

The following table lists the configurable parameters of the latest Azure Blob Storage CSI driver chart and default values.

| Parameter                                         | Description                                                | Default                                                           |
|---------------------------------------------------|------------------------------------------------------------|-------------------------------------------------------------------|
| `image.blob.repository`                       | blob-csi-driver docker image                           | mcr.microsoft.com/k8s/csi/blob-csi                            |
| `image.blob.tag`                              | blob-csi-driver docker image tag                       | latest                                                            |
| `image.blob.pullPolicy`                       | blob-csi-driver image pull policy                      | IfNotPresent                                                      |
| `image.csiProvisioner.repository`                 | csi-provisioner docker image                               | mcr.microsoft.com/oss/kubernetes-csi/csi-provisioner              |
| `image.csiProvisioner.tag`                        | csi-provisioner docker image tag                           | v1.4.0                                                            |
| `image.csiProvisioner.pullPolicy`                 | csi-provisioner image pull policy                          | IfNotPresent                                                      |
| `image.livenessProbe.repository`                  | liveness-probe docker image                                | mcr.microsoft.com/oss/kubernetes-csi/livenessprobe                |
| `image.livenessProbe.tag`                         | liveness-probe docker image tag                            | v1.1.0                                                            |
| `image.livenessProbe.pullPolicy`                  | liveness-probe image pull policy                           | IfNotPresent                                                      |
| `image.nodeDriverRegistrar.repository`            | csi-node-driver-registrar docker image                     | mcr.microsoft.com/oss/kubernetes-csi/csi-node-driver-registrar    |
| `image.nodeDriverRegistrar.tag`                   | csi-node-driver-registrar docker image tag                 | v2.0.1                                                            |
| `image.nodeDriverRegistrar.pullPolicy`            | csi-node-driver-registrar image pull policy                | IfNotPresent                                                      |
| `imagePullSecrets`                                | Specify docker-registry secret names as an array           | [] (does not add image pull secrets to deployed pods)         |      
| `serviceAccount.create`                           | whether create service account of csi-blob-controller  | true                                                              |
| `rbac.create`                                     | whether create rbac of csi-blob-controller             | true                                                              |
| `controller.replicas`                             | the replicas of csi-blob-controller                    | 2                                                                 |
| `controller.metricsPort`                          | metrics port of csi-blob-controller                    | 29634                                                             |
| `controller.runOnMaster`                          | run controller on master node                          | false                                                             |
| `node.metricsPort`                                | metrics port of csi-blob-node                          | 29635                                                                |
| `kubelet.linuxPath`                               | configure the kubelet path for Linux node                  | `/var/lib/kubelet`                                                |
| `cloud`                                           | the cloud environment the driver is running on             | AzurePublicCloud                                                  |

## troubleshooting
 - Add `--wait -v=5 --debug` in `helm install` command to get detailed error
 - Use `kubectl describe` to acquire more info
