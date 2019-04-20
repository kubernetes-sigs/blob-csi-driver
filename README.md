# blobfuse CSI driver for Kubernetes
![TravisCI](https://travis-ci.com/csi-driver/blobfuse-csi-driver.svg?branch=master)
[![Coverage Status](https://coveralls.io/repos/github/csi-driver/blobfuse-csi-driver/badge.svg?branch=master)](https://coveralls.io/github/csi-driver/blobfuse-csi-driver?branch=master)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcsi-driver%2Fblobfuse-csi-driver.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcsi-driver%2Fblobfuse-csi-driver?ref=badge_shield)

**WARNING**: This driver is in ALPHA currently. Do NOT use this driver in a production environment in its current state.

### About
This driver allows Kubernetes to use [Azure Blob storage](https://github.com/Azure/azure-storage-fuse), csi plugin name: `blobfuse.csi.azure.com`

### Project Status
Status: Aplha

### Container Images & CSI Compatibility:
|Blobfuse CSI Driver Version  | Image                                                | v1.0.0 |
|-------------------------------|----------------------------------------------------|--------|
|v0.1.0-alpha                   |mcr.microsoft.com/k8s/csi/blobfuse-csi:v0.1.0-alpha | yes    |
|master branch                  |mcr.microsoft.com/k8s/csi/blobfuse-csi:latest       | yes    |

### Kubernetes Compatibility
| Blobfuse CSI Driver\Kubernetes Version   | 1.13+ | 
|------------------------------------------|-------|
| v0.1.0-alpha                             | yes   |
| master branch                            | yes   |

### Driver parameters
Please refer to [`blobfuse.csi.azure.com` driver parameters](./docs/driver-parameters.md)
 > storage class `blobfuse.csi.azure.com` parameters are compatible with built-in [blobfuse](https://kubernetes.io/docs/concepts/storage/volumes/#blobfuse) plugin

### Prerequisite
 - The driver initialization depends on a [Cloud provider config file](https://github.com/kubernetes/cloud-provider-azure/blob/master/docs/cloud-provider-config.md), usually it's `/etc/kubernetes/azure.json` on all k8s nodes deployed by AKS or aks-engine, here is an [azure.json example](./deploy/example/azure.json)

### Install blobfuse CSI driver on a kubernetes cluster
Please refer to [install blobfuse csi driver](https://github.com/csi-driver/blobfuse-csi-driver/blob/master/docs/install-blobfuse-csi-driver.md)

## E2E Usage example
### 1. create a pod with csi blobfuse driver mount on linux
#### Blobfuse Dynamic Provisioning
 - Create an blobfuse CSI storage class
```sh
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/storageclass-blobfuse-csi.yaml
```

 - Create an blobfuse CSI PVC
```sh
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pvc-blobfuse-csi.yaml
```

#### Blobfuse Static Provisioning(use an existing storage container)
##### Option#1: use existing credentials in k8s cluster
 > make sure the existing credentials in k8s cluster(e.g. service principal, msi) could access the specified storage account
 - Download an blobfuse CSI storage class, edit `resourceGroup`, `storageAccount`, `containerName` to use existing storage container
```sh
wget -O storageclass-blobfuse-csi-existing-container.yaml https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/storageclass-blobfuse-csi-existing-container.yaml
vi storageclass-blobfuse-csi-existing-container.yaml
kubectl create -f storageclass-blobfuse-csi-existing-container.yaml
```

 - Create an blobfuse CSI PVC
```sh
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pvc-blobfuse-csi.yaml
```

##### Option#2: provide storage account name and key
 - Use `kubectl create secret` to create `azure-secret` with existing storage account name and key
```
kubectl create secret generic azure-secret --from-literal accountname=NAME --from-literal accountkey="KEY" --type=Opaque
```

 - Create an blobfuse CSI PV, download `pv-blobfuse-csi.yaml` file and edit `containerName` in `volumeAttributes`
```
wget https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pv-blobfuse-csi.yaml
vi pv-blobfuse-csi.yaml
kubectl create -f pv-blobfuse-csi.yaml
```

 - Create an blobfuse CSI PVC which would be bound to the above PV
```
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pvc-blobfuse-csi-static.yaml
```

#### 2. Validate PVC status and create an nginx pod
 - make sure pvc is created and in `Bound` status finally
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
 - [Kubernetes CSI Documentation](https://kubernetes-csi.github.io/docs/Home.html)
 - [Analysis of the CSI Spec](https://blog.thecodeteam.com/2017/11/03/analysis-csi-spec/)
 - [CSI Drivers](https://github.com/kubernetes-csi/drivers)
 - [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec)
