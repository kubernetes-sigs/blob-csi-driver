# Azure Blob Storage CSI driver for Kubernetes
[![Travis](https://travis-ci.org/kubernetes-sigs/blob-csi-driver.svg)](https://travis-ci.org/kubernetes-sigs/blob-csi-driver)
[![Coverage Status](https://coveralls.io/repos/github/kubernetes-sigs/blob-csi-driver/badge.svg?branch=master)](https://coveralls.io/github/kubernetes-sigs/blob-csi-driver?branch=master)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fkubernetes-sigs%2Fblob-csi-driver.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fkubernetes-sigs%2Fblob-csi-driver?ref=badge_shield)

### About
This driver allows Kubernetes to access Azure Storage through one of following methods:
 - [azure-storage-fuse](https://github.com/Azure/azure-storage-fuse)
 - [NFSv3](https://docs.microsoft.com/en-us/azure/storage/blobs/network-file-system-protocol-support)

csi plugin name: `blob.csi.azure.com`

#### Breaking change
Since `v0.7.0`, driver name changed from `blobfuse.csi.azure.com` to `blob.csi.azure.com`, volume created by `v0.6.0`(or prior version) could not be mounted by `v0.7.0` driver. If you have volumes created by `v0.6.0` version, just keep the driver running in your cluster.

### Container Images & Kubernetes Compatibility:
|driver version  |Image                                      | 1.15+  | built-in blobfuse version |
|----------------|-------------------------------------------|--------|---------------------------|
|master branch   |mcr.microsoft.com/k8s/csi/blob-csi:latest  | yes    | 1.3.4                     |
|v0.9.0          |mcr.microsoft.com/k8s/csi/blob-csi:v0.9.0  | yes    | 1.3.4                     |
|v0.8.0          |mcr.microsoft.com/k8s/csi/blob-csi:v0.8.0  | yes    | 1.3.1                     |
|v0.7.0          |mcr.microsoft.com/k8s/csi/blob-csi:v0.7.0  | yes    | 1.2.4                     |

### Driver parameters
Please refer to `blob.csi.azure.com` [driver parameters](./docs/driver-parameters.md)

### Prerequisite
 - The driver depends on [cloud provider config file](https://github.com/kubernetes/cloud-provider-azure/blob/master/docs/cloud-provider-config.md), usually it's `/etc/kubernetes/azure.json` on all kubernetes nodes deployed by [AKS](https://docs.microsoft.com/en-us/azure/aks/) or [aks-engine](https://github.com/Azure/aks-engine), here is [azure.json example](./deploy/example/azure.json).
 > To specify a different cloud provider config file, create `azure-cred-file` configmap before driver installation, e.g. for OpenShift, it's `/etc/kubernetes/cloud.conf` (make sure config file path is in the `volumeMounts.mountPath`)
 > ```console
 > kubectl create configmap azure-cred-file --from-literal=path="/etc/kubernetes/cloud.conf" --from-literal=path-windows="C:\\k\\cloud.conf" -n kube-system
 > ```
 - This driver also supports [read cloud config from kuberenetes secret](./docs/read-from-secret.md).
 - If cluster identity is [Managed Service Identity(MSI)](https://docs.microsoft.com/en-us/azure/aks/use-managed-identity), make sure user assigned identity has `Contributor` role on node resource group

### Install Azure Blob Storage CSI driver on a kubernetes cluster
Please refer to [install Azure Blob Storage CSI driver](https://github.com/kubernetes-sigs/blob-csi-driver/blob/master/docs/install-blob-csi-driver.md)

### Usage
 - [Basic usage](./deploy/example/e2e_usage.md)
 - [NFSv3](./deploy/example/nfs)
 
### Troubleshooting
 - [CSI driver troubleshooting guide](./docs/csi-debug.md)

## Kubernetes Development
Please refer to [development guide](./docs/csi-dev.md)

### View CI Results
Check testgrid [provider-azure-blobfuse-csi-driver](https://testgrid.k8s.io/provider-azure-blobfuse-csi-driver) dashboard.

### Links
 - [azure-storage-fuse](https://github.com/Azure/azure-storage-fuse)
 - [Kubernetes CSI Documentation](https://kubernetes-csi.github.io/docs/)
 - [CSI Drivers](https://github.com/kubernetes-csi/drivers)
 - [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec)
 - [Blobfuse FlexVolume driver](https://github.com/Azure/kubernetes-volume-drivers/tree/master/flexvolume/blobfuse)
