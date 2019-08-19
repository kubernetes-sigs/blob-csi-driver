# Installation with Helm

Quick start instructions for the setup and configuration of blobfuse CSI driver using Helm.

## Prerequisites

1. [install Helm Client](https://helm.sh/docs/using_helm/#installing-the-helm-client)

2. [initialize Helm and install Tiller](https://helm.sh/docs/using_helm/#initialize-helm-and-install-tiller)

## Install BlobFuse via `helm install`

```console
$ cd $GOPATH/src/github.com/csi-driver/blobfuse-csi-driver/charts/latest
$ helm package blobfuse-csi-driver
$ helm install blobfuse-csi-driver-latest.tgz --name blobfuse-csi-driver --namespace kube-system
```

## Uninstall

```console
$ helm delete --purge blobfuse-csi-driver
```
