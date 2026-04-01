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
  --from-literal=path-windows="C:\k