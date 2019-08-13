# blobfuse CSI driver for Kubernetes
![TravisCI](https://travis-ci.com/csi-driver/blobfuse-csi-driver.svg?branch=master)
[![Coverage Status](https://coveralls.io/repos/github/csi-driver/blobfuse-csi-driver/badge.svg?branch=master)](https://coveralls.io/github/csi-driver/blobfuse-csi-driver?branch=master)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcsi-driver%2Fblobfuse-csi-driver.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcsi-driver%2Fblobfuse-csi-driver?ref=badge_shield)

### About
This driver allows Kubernetes to use [azure-storage-fuse](https://github.com/Azure/azure-storage-fuse), csi plugin name: `blobfuse.csi.azure.com`

### Project Status
Status: Beta

### Container Images & CSI Compatibility:
|Blobfuse CSI Driver Version  | Image                                                | v1.0.0 |
|-------------------------------|----------------------------------------------------|--------|
|v0.1.0-alpha                   |mcr.microsoft.com/k8s/csi/blobfuse-csi:v0.1.0-alpha | yes    |
|v0.2.0                         |mcr.microsoft.com/k8s/csi/blobfuse-csi:v0.2.0       | yes    |
|master branch                  |mcr.microsoft.com/k8s/csi/blobfuse-csi:latest       | yes    |

### Kubernetes Compatibility
| Blobfuse CSI Driver\Kubernetes Version   | 1.13+ |
|------------------------------------------|-------|
| v0.1.0-alpha                             | yes   |
| v0.2.0                                   | yes   |
| master branch                            | yes   |

### Driver parameters
Please refer to `blobfuse.csi.azure.com` [driver parameters](./docs/driver-parameters.md)
 > storage class `blobfuse.csi.azure.com` parameters are compatible with built-in [blobfuse](https://kubernetes.io/docs/concepts/storage/volumes/#blobfuse) plugin

### Prerequisite
 - The driver initialization depends on a [Cloud provider config file](https://github.com/kubernetes/cloud-provider-azure/blob/master/docs/cloud-provider-config.md), usually it's `/etc/kubernetes/azure.json` on all kubernetes nodes deployed by AKS or aks-engine, here is an [azure.json example](./deploy/example/azure.json)
 > if cluster is based on Managed Service Identity(MSI), make sure all agent nodes have `Contributor` role for current resource group

### Install blobfuse CSI driver on a kubernetes cluster
Please refer to [install blobfuse csi driver](https://github.com/csi-driver/blobfuse-csi-driver/blob/master/docs/install-blobfuse-csi-driver.md)

## E2E Usage example
create a pod with blobfuse mount on linux
### Dynamic Provisioning (create storage account and container automatically by blobfuse driver)
 - Create a blobfuse CSI storage class
```sh
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/storageclass-blobfuse-csi-mountoptions.yaml
```

 - Create a blobfuse CSI PVC
```sh
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pvc-blobfuse-csi.yaml
```

### Static Provisioning(use an existing storage account)
#### Option#1: use existing credentials in k8s cluster
 > make sure the existing credentials in k8s cluster(e.g. service principal, msi) could access the specified storage account
 - Download a blobfuse CSI storage class, edit `resourceGroup`, `storageAccount`, `containerName` in storage class
```sh
wget https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/storageclass-blobfuse-csi-existing-container.yaml
vi storageclass-blobfuse-csi-existing-container.yaml
kubectl create -f storageclass-blobfuse-csi-existing-container.yaml
```

 - Create a blobfuse CSI PVC
```sh
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pvc-blobfuse-csi.yaml
```

#### Option#2: provide storage account name and key
 - Use `kubectl create secret` to create `azure-secret` with existing storage account name and key
```
kubectl create secret generic azure-secret --from-literal accountname=NAME --from-literal accountkey="KEY" --type=Opaque
```

 - Create a blobfuse CSI PV, download `pv-blobfuse-csi.yaml` file and edit `containerName` in `volumeAttributes`
```sh
wget https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pv-blobfuse-csi.yaml
vi pv-blobfuse-csi.yaml
kubectl create -f pv-blobfuse-csi.yaml
```

 - Create a blobfuse CSI PVC which would be bound to the above PV
```
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pvc-blobfuse-csi-static.yaml
```

#### 2. Validate PVC status and create an nginx pod
 > make sure pvc is created and in `Bound` status
```
watch kubectl describe pvc pvc-blobfuse
```

 - create a pod with blobfuse CSI PVC
```
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/nginx-pod-blobfuse.yaml
```

#### 3. enter the pod container to do validation
 - watch the status of pod until its Status changed from `Pending` to `Running` and then enter the pod container
```sh
$ watch kubectl describe po nginx-blobfuse
$ kubectl exec -it nginx-blobfuse -- bash
Filesystem      Size  Used Avail Use% Mounted on
...
blobfuse         30G  8.9G   21G  31% /mnt/blobfuse
/dev/sda1        30G  8.9G   21G  31% /etc/hosts
...
```
In the above example, there is a `/mnt/blobfuse` directory mounted as dysk filesystem.

## Kubernetes Development
Please refer to [development guide](./docs/csi-dev.md)


### Links
 - [azure-storage-fuse](https://github.com/Azure/azure-storage-fuse)
 - [Kubernetes CSI Documentation](https://kubernetes-csi.github.io/docs/Home.html)
 - [Analysis of the CSI Spec](https://blog.thecodeteam.com/2017/11/03/analysis-csi-spec/)
 - [CSI Drivers](https://github.com/kubernetes-csi/drivers)
 - [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec)
