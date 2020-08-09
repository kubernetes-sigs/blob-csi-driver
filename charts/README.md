# Installation with Helm 3

Quick start instructions for the setup and configuration of Azure Blob Storage CSI driver using Helm.

## Prerequisites

1. [install Helm Client 3.0+ ](https://helm.sh/docs/intro/quickstart/#install-helm)

## Install latest CSI Driver via `helm install`

```console
$ cd $GOPATH/src/sigs.k8s.io/blob-csi-driver/charts/latest
$ helm package blob-csi-driver
$ helm install blob-csi-driver blob-csi-driver-latest.tgz --namespace kube-system
```
  
## Install CSI Driver released version using Helm repository

```console
$ helm repo add blob-csi-driver https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/charts
$ helm install blob-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system
```
  
### Search for different versions of charts available
```console
$ helm search repo -l blob-csi-driver/
```  
### Install a specific version of Helm chart
Specify the version of the chart to be installed using the `--version` parameter. 
```console
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system --version v0.6.0
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
| `image.csiAttacher.repository`                    | csi-attacher docker image                                  | mcr.microsoft.com/oss/kubernetes-csi/csi-attacher                 |
| `image.csiAttacher.tag`                           | csi-attacher docker image tag                              | v2.2.0                                                            |
| `image.csiAttacher.pullPolicy`                    | csi-attacher image pull policy                             | IfNotPresent                                                      |                                                  |
| `image.livenessProbe.repository`                  | liveness-probe docker image                                | mcr.microsoft.com/oss/kubernetes-csi/livenessprobe                |
| `image.livenessProbe.tag`                         | liveness-probe docker image tag                            | v1.1.0                                                            |
| `image.livenessProbe.pullPolicy`                  | liveness-probe image pull policy                           | IfNotPresent                                                      |
| `image.nodeDriverRegistrar.repository`            | csi-node-driver-registrar docker image                     | mcr.microsoft.com/oss/kubernetes-csi/csi-node-driver-registrar    |
| `image.nodeDriverRegistrar.tag`                   | csi-node-driver-registrar docker image tag                 | v1.2.0                                                            |
| `image.nodeDriverRegistrar.pullPolicy`            | csi-node-driver-registrar image pull policy                | IfNotPresent                                                      |
| `serviceAccount.create`                           | whether create service account of csi-blob-controller  | true                                                              |
| `rbac.create`                                     | whether create rbac of csi-blob-controller             | true                                                              |
| `controller.replicas`                             | the replicas of csi-blob-controller                    | 2                                                                 |
## Troubleshooting

If there are some errors when using helm to install, follow the steps to debug:

1. Add `--wait -v=5 --debug` in `helm install` command.
2. Then the error pods  can be located.
3. Use `kubectl describe ` to acquire more info.
4. Check the related resource of the pod, such as serviceaacount, rbac, etc.

