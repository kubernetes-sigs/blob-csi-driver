#### Breaking change
Since `v0.7.0`, driver name changed from `blobfuse.csi.azure.com` to `blob.csi.azure.com`, volume created by `v0.6.0`(or prior version) could not be mounted by `v0.7.0` driver. If you have volumes created by `v0.6.0` version, just keep the driver running in your cluster.

# Installation with Helm 3

Quick start instructions for the setup and configuration of Azure Blob Storage CSI driver using Helm.

## Prerequisites

 - [install Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

## Install latest CSI Driver via `helm install`

```console
$ cd $GOPATH/src/sigs.k8s.io/blob-csi-driver/charts/latest
$ helm package blob-csi-driver
$ helm install blob-csi-driver blob-csi-driver-latest.tgz --namespace kube-system
```
  
## Install latest CSI Driver on Azure Stack via `helm install`

```console
$ cd $GOPATH/src/sigs.k8s.io/blob-csi-driver
$ helm install blob-csi-driver ./charts/latest/blob-csi-driver --namespace kube-system --set cloud=AzureStackCloud
```

### Install a specific version

```console
$ helm repo add blob-csi-driver https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/charts
$ helm install blob-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system --version v0.11.0
```
  
### Search for all available chart versions
```console
$ helm search repo -l blob-csi-driver/
```  

## Uninstall

```console
$ helm uninstall blob-csi-driver -n kube-system
```  
## The Latest Helm Chart Configuration

The following table lists the configurable parameters of the latest Azure Blob Storage CSI driver chart and their default values.

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

## Troubleshooting
 - Add `--wait -v=5 --debug` in `helm install` command to get detailed error
 - Use `kubectl describe` to acquire more info
