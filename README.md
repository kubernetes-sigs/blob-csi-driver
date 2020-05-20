# Blobfuse CSI driver for Kubernetes
[![Coverage Status](https://coveralls.io/repos/github/kubernetes-sigs/blobfuse-csi-driver/badge.svg?branch=master)](https://coveralls.io/github/kubernetes-sigs/blobfuse-csi-driver?branch=master)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fkubernetes-sigs%2Fblobfuse-csi-driver.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fkubernetes-sigs%2Fblobfuse-csi-driver?ref=badge_shield)

### About
This driver allows Kubernetes to use [azure-storage-fuse](https://github.com/Azure/azure-storage-fuse), csi plugin name: `blobfuse.csi.azure.com`

### Container Images & Kubernetes Compatibility:
|Blobfuse CSI Driver Version    | Image                                              | 1.14+  |
|-------------------------------|----------------------------------------------------|--------|
|master branch                  |mcr.microsoft.com/k8s/csi/blobfuse-csi:latest       | yes    |
|v0.4.0                         |mcr.microsoft.com/k8s/csi/blobfuse-csi:v0.4.0       | yes    |
|v0.3.0                         |mcr.microsoft.com/k8s/csi/blobfuse-csi:v0.3.0       | yes    |

### Driver parameters
Please refer to `blobfuse.csi.azure.com` [driver parameters](./docs/driver-parameters.md)
 > storage class `blobfuse.csi.azure.com` parameters are compatible with built-in [blobfuse](https://kubernetes.io/docs/concepts/storage/volumes/#blobfuse) plugin

### Prerequisite
 - The driver depends on [cloud provider config file](https://github.com/kubernetes/cloud-provider-azure/blob/master/docs/cloud-provider-config.md), usually it's `/etc/kubernetes/azure.json` on all kubernetes nodes deployed by [AKS](https://docs.microsoft.com/en-us/azure/aks/) or [aks-engine](https://github.com/Azure/aks-engine), here is [azure.json example](./deploy/example/azure.json).
 > To specify a different cloud provider config file, create `azure-cred-file` configmap before driver installation, e.g. for OpenShift, it's `/etc/kubernetes/cloud.conf` (make sure config file path is in the `volumeMounts.mountPath`)
 > ```console
 > kubectl create configmap azure-cred-file --from-literal=path="/etc/kubernetes/cloud.conf" --from-literal=path-windows="C:\\k\\cloud.conf" -n kube-system
 > ```
 - This driver also supports [read cloud config from kuberenetes secret](./docs/read-from-secret.md).
 - If cluster identity is [Managed Service Identity(MSI)](https://docs.microsoft.com/en-us/azure/aks/use-managed-identity), make sure user assigned identity has `Contributor` role on node resource group

### Install blobfuse CSI driver on a kubernetes cluster
Please refer to [install blobfuse csi driver](https://github.com/kubernetes-sigs/blobfuse-csi-driver/blob/master/docs/install-blobfuse-csi-driver.md)

### Examples
 - [Basic usage](./deploy/example/e2e_usage.md)
 
### Troubleshooting
 - [CSI driver troubleshooting guide](./docs/csi-debug.md)

## Kubernetes Development
Please refer to [development guide](./docs/csi-dev.md)


### Links
 - [azure-storage-fuse](https://github.com/Azure/azure-storage-fuse)
 - [blobfuse flexvolume driver](https://github.com/Azure/kubernetes-volume-drivers/tree/master/flexvolume/blobfuse)
 - [Kubernetes CSI Documentation](https://kubernetes-csi.github.io/docs/Home.html)
 - [Analysis of the CSI Spec](https://blog.thecodeteam.com/2017/11/03/analysis-csi-spec/)
 - [CSI Drivers](https://github.com/kubernetes-csi/drivers)
 - [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec)
