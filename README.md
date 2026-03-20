# Azure Blob Storage CSI Driver for Kubernetes

![linux build status](https://github.com/kubernetes-sigs/blob-csi-driver/actions/workflows/linux.yaml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/kubernetes-sigs/blob-csi-driver/badge.svg?branch=master)](https://coveralls.io/github/kubernetes-sigs/blob-csi-driver?branch=master)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fkubernetes-sigs%2Fblob-csi-driver.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fkubernetes-sigs%2Fblob-csi-driver?ref=badge_shield)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/blob-csi-driver)](https://artifacthub.io/packages/search?repo=blob-csi-driver)

## About

This driver allows Kubernetes to access [Azure Blob Storage](https://azure.microsoft.com/en-us/services/storage/blobs/) through one of the following methods:

- [azure-storage-fuse](https://github.com/Azure/azure-storage-fuse)
- [NFSv3](https://docs.microsoft.com/en-us/azure/storage/blobs/network-file-system-protocol-support)

- **CSI plugin name:** `blob.csi.azure.com`
- **Project status:** GA

> [!NOTE]
> Deploying this driver manually is not an officially supported Microsoft product. For a fully managed and supported experience on Kubernetes, use [AKS with the managed Blob CSI driver](https://learn.microsoft.com/en-us/azure/aks/azure-blob-csi).

## Container Images & Kubernetes Compatibility

| Driver Version | Image                                                            | Supported K8s Version |
|----------------|------------------------------------------------------------------|-----------------------|
| master branch  | `mcr.microsoft.com/k8s/csi/blob-csi:latest`                     | 1.21+                 |
| v1.27.3        | `mcr.microsoft.com/oss/v2/kubernetes-csi/blob-csi:v1.27.3`      | 1.21+                 |
| v1.26.10       | `mcr.microsoft.com/oss/v2/kubernetes-csi/blob-csi:v1.26.10`     | 1.21+                 |
| v1.25.6        | `mcr.microsoft.com/oss/kubernetes-csi/blob-csi:v1.25.6`         | 1.21+                 |

## Driver Parameters

Please refer to [`blob.csi.azure.com` driver parameters](./docs/driver-parameters.md).

## Prerequisites

### Option 1: Provide cloud provider config with Azure credentials

This option depends on a [cloud provider config file](https://github.com/kubernetes/cloud-provider-azure/blob/master/docs/cloud-provider-config.md) ([config example](./deploy/example/azure.json)). Config file paths on different clusters:

| Platform | Config Path |
|----------|-------------|
| [AKS](https://docs.microsoft.com/en-us/azure/aks/), [CAPZ](https://github.com/kubernetes-sigs/cluster-api-provider-azure), [aks-engine](https://github.com/Azure/aks-engine) | `/etc/kubernetes/azure.json` |
| Azure Red Hat OpenShift | `/etc/kubernetes/cloud.conf` |

<details>
<summary>Specify a different config file path via ConfigMap</summary>

Create the ConfigMap `azure-cred-file` before the driver starts up:

```bash
kubectl create configmap azure-cred-file \
  --from-literal=path="/etc/kubernetes/cloud.conf" \
  --from-literal=path-windows="C:\\k\\cloud.conf" \
  -n kube-system
```

</details>

- Cloud provider config can also be specified via a Kubernetes Secret — see [details](./docs/read-from-secret.md).
- Ensure the identity used by the driver has the `Contributor` role on the node resource group and virtual network resource group.

### Option 2: Bring your own storage account

This option does not depend on the cloud provider config file and supports cross-subscription and on-premise cluster scenarios. Refer to [detailed steps](./deploy/example/e2e_usage.md#option2-bring-your-own-storage-account).

## Installation

Install the driver on a Kubernetes cluster:

- Install by [Helm charts](./charts)
- Install by [kubectl](./docs/install-blob-csi-driver.md)

**Install open source CSI driver:**

| Platform | Guide |
|----------|-------|
| AKS | [Install on AKS](./docs/install-driver-on-aks.md) |
| Azure Red Hat OpenShift | [Install on ARO](https://github.com/ezYakaEagle442/aro-pub-storage/blob/master/setup-store-CSI-driver-azure-blob.md) |

**Install managed CSI driver:**

| Platform | Guide |
|----------|-------|
| AKS | [AKS managed Blob CSI driver](https://learn.microsoft.com/en-us/azure/aks/azure-blob-csi) |

<details>
<summary>Install a specific version of blobfuse v2</summary>

Execute the following command to install a specific version of blobfuse v2 once the driver is running on the agent node:

```console
kubectl patch daemonset csi-blob-node -n kube-system -p '{"spec":{"template":{"spec":{"initContainers":[{"env":[{"name":"INSTALL_BLOBFUSE2","value":"true"},{"name":"BLOBFUSE2_VERSION","value":"2.5.2"}],"name":"install-blobfuse-proxy"}]}}}}'
```

To install a lower version of blobfuse2 than the current version, add `--allow-downgrades` to the `BLOBFUSE2_VERSION` value:

```console
kubectl patch daemonset csi-blob-node -n kube-system -p '{"spec":{"template":{"spec":{"initContainers":[{"env":[{"name":"INSTALL_BLOBFUSE2","value":"true"},{"name":"BLOBFUSE2_VERSION","value":"2.3.0 --allow-downgrades"}],"name":"install-blobfuse-proxy"}]}}}}'
```

</details>

## Examples

- [Basic usage](./deploy/example/e2e_usage.md)

## Features

- [NFSv3](./deploy/example/nfs)
- [fsGroupPolicy](./deploy/example/fsgroup)
- [Volume cloning](./deploy/example/cloning)
- [Mount with workload identity](./docs/workload-identity-static-pv-mount.md)
- [Mount with managed identity](./deploy/example/blobfuse-mi)

## Troubleshooting

- [CSI driver troubleshooting guide](./docs/csi-debug.md)

## Support

Please see our [support policy](support.md).

## Limitations

Please refer to [Azure Blob Storage CSI Driver Limitations](./docs/limitations.md).

## Development

Please refer to the [development guide](./docs/csi-dev.md).

## CI Results

Check the TestGrid [provider-azure-blobfuse-csi-driver](https://testgrid.k8s.io/provider-azure-blobfuse-csi-driver) dashboard.

## Links

- [azure-storage-fuse](https://github.com/Azure/azure-storage-fuse)
- [Kubernetes CSI Documentation](https://kubernetes-csi.github.io/docs/)
- [CSI Drivers](https://github.com/kubernetes-csi/drivers)
- [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec)
- [Blobfuse2 Known Issues](https://github.com/Azure/azure-storage-fuse/wiki/Blobfuse2-Known-issues)
