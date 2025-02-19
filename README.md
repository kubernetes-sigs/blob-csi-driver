# Azure Blob Storage CSI driver for Kubernetes
![linux build status](https://github.com/kubernetes-sigs/blob-csi-driver/actions/workflows/linux.yaml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/kubernetes-sigs/blob-csi-driver/badge.svg?branch=master)](https://coveralls.io/github/kubernetes-sigs/blob-csi-driver?branch=master)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fkubernetes-sigs%2Fblob-csi-driver.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fkubernetes-sigs%2Fblob-csi-driver?ref=badge_shield)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/blob-csi-driver)](https://artifacthub.io/packages/search?repo=blob-csi-driver)

### About
This driver allows Kubernetes to access Azure Storage through one of following methods:
 - [azure-storage-fuse](https://github.com/Azure/azure-storage-fuse)
 - [NFSv3](https://docs.microsoft.com/en-us/azure/storage/blobs/network-file-system-protocol-support)
 
Disclaimer: Deploying this driver manually is not an officially supported Microsoft product. For a fully managed and supported experience on Kubernetes, use [AKS with the managed blob csi driver](https://learn.microsoft.com/en-us/azure/aks/azure-blob-csi).

#### csi plugin name: `blob.csi.azure.com`

### Project status: GA

### Container Images & Kubernetes Compatibility:
|driver version  |Image                                                 | supported k8s version |
|----------------|------------------------------------------------------|-----------------------|
|master branch   |mcr.microsoft.com/k8s/csi/blob-csi:latest             | 1.21+                 |
|v1.25.2         |mcr.microsoft.com/oss/kubernetes-csi/blob-csi:v1.25.2 | 1.21+                 |
|v1.24.3         |mcr.microsoft.com/oss/kubernetes-csi/blob-csi:v1.24.3 | 1.21+                 |
|v1.23.7         |mcr.microsoft.com/oss/kubernetes-csi/blob-csi:v1.23.7 | 1.21+                 |
|v1.22.8         |mcr.microsoft.com/oss/kubernetes-csi/blob-csi:v1.22.8 | 1.21+                 |

### Driver parameters
Please refer to `blob.csi.azure.com` [driver parameters](./docs/driver-parameters.md)

### Prerequisites
#### Option#1: Provide cloud provider config with Azure credentials
 - This option depends on [cloud provider config file](https://github.com/kubernetes/cloud-provider-azure/blob/master/docs/cloud-provider-config.md) (here is [config example](./deploy/example/azure.json)), config file path on different clusters:
   - [AKS](https://docs.microsoft.com/en-us/azure/aks/), [capz](https://github.com/kubernetes-sigs/cluster-api-provider-azure), [aks-engine](https://github.com/Azure/aks-engine): `/etc/kubernetes/azure.json`
   - Azure RedHat OpenShift: `/etc/kubernetes/cloud.conf`
 - <details> <summary>specify a different config file path via configmap</summary></br>create configmap "azure-cred-file" before driver starts up</br><pre>kubectl create configmap azure-cred-file --from-literal=path="/etc/kubernetes/cloud.conf" --from-literal=path-windows="C:\\k\\cloud.conf" -n kube-system</pre></details>
 - Cloud provider config can also be specified via kubernetes secret, check details [here](./docs/read-from-secret.md)
 - Make sure identity used by driver has `Contributor` role on node resource group and virtual network resource group

#### Option#2: Bring your own storage account
This option does not depend on cloud provider config file, supports cross subscription and on-premise cluster scenario. Refer to [detailed steps](./deploy/example/e2e_usage.md#option2-bring-your-own-storage-account).

### Install driver on a Kubernetes cluster
> Note: this feature is only available in v1.19.5, v1.21.1 and later versions.
>
> Execute following command to install a specific version of blobfuse v2 once driver is running on the agent node:
> ```console
> kubectl patch daemonset csi-blob-node -n kube-system -p '{"spec":{"template":{"spec":{"initContainers":[{"env":[{"name":"INSTALL_BLOBFUSE2","value":"true"},{"name":"BLOBFUSE2_VERSION","value":"2.4.1"}],"name":"install-blobfuse-proxy"}]}}}}'
> ```
>
> Execute following command to install a specific version of blobfuse v1 once driver is running on the agent node:
> ```console
> kubectl patch daemonset csi-blob-node -n kube-system -p '{"spec":{"template":{"spec":{"initContainers":[{"env":[{"name":"INSTALL_BLOBFUSE","value":"true"},{"name":"BLOBFUSE_VERSION","value":"1.4.5"}],"name":"install-blobfuse-proxy"}]}}}}'
> ```

 - install by [helm charts](./charts)
 - install by [kubectl](./docs/install-blob-csi-driver.md)
 - install open source CSI driver on following platforms:
   - [AKS](./docs/install-driver-on-aks.md)
   - [Azure RedHat OpenShift](https://github.com/ezYakaEagle442/aro-pub-storage/blob/master/setup-store-CSI-driver-azure-blob.md)
 - install managed CSI driver on following platforms:
   - [AKS](https://learn.microsoft.com/en-us/azure/aks/azure-blob-csi)

### Examples
 - [Basic usage](./deploy/example/e2e_usage.md)

### Usage
 - [NFSv3](./deploy/example/nfs)
 - [fsGroupPolicy](./deploy/example/fsgroup)
 - [Volume cloning](./deploy/example/cloning)
 - [Workload identity](./docs/workload-identity-static-pv-mount.md)
 - [Mount Azure blob storage with managed identity](./deploy/example/blobfuse-mi)

### Troubleshooting
 - [CSI driver troubleshooting guide](./docs/csi-debug.md)

### Support
 - Please see our [support policy][support-policy]

### Limitations
 - Please refer to [Azure Blob Storage CSI Driver Limitations](./docs/limitations.md)

## Kubernetes Development
 - Please refer to [development guide](./docs/csi-dev.md)

### View CI Results
 - Check testgrid [provider-azure-blobfuse-csi-driver](https://testgrid.k8s.io/provider-azure-blobfuse-csi-driver) dashboard.

### Links
 - [azure-storage-fuse](https://github.com/Azure/azure-storage-fuse)
 - [Kubernetes CSI Documentation](https://kubernetes-csi.github.io/docs/)
 - [CSI Drivers](https://github.com/kubernetes-csi/drivers)
 - [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec)
 - [Blobfuse2 Known issues](https://github.com/Azure/azure-storage-fuse/wiki/Blobfuse2-Known-issues)

[support-policy]: support.md
