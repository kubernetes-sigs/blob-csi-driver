# blobfuse CSI driver for Kubernetes (Under heavy development now)
![TravisCI](https://travis-ci.com/csi-driver/blobfuse-csi-driver.svg?branch=master)
[![Coverage Status](https://coveralls.io/repos/github/csi-driver/blobfuse-csi-driver/badge.svg?branch=master)](https://coveralls.io/github/csi-driver/blobfuse-csi-driver?branch=master)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fcsi-driver%2Fblobfuse-csi-driver.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fcsi-driver%2Fblobfuse-csi-driver?ref=badge_shield)

**WARNING**: This driver is in ALPHA currently. Do NOT use this driver in a production environment in its current state.

### About
This driver allows Kubernetes to use [blobfuse](https://docs.microsoft.com/en-us/azure/storage/files/storage-files-introduction) volume, csi plugin name: `blobfuse.csi.azure.com`

### Project Status
Status: Aplha

### Container Images & CSI Compatibility:
|Azure File CSI Driver Version  | Image                                              | v0.3.0| v1.0.0 |
|-------------------------------|----------------------------------------------------|-------|--------|
|v0.1.0-alpha                   |mcr.microsoft.com/k8s/csi/blobfuse-csi:v0.1.0-alpha| no    | yes    |
|master branch                  |mcr.microsoft.com/k8s/csi/blobfuse-csi:latest      | no    | yes    |

### Kubernetes Compatibility
| Azure File CSI Driver\Kubernetes Version | 1.12 | 1.13+ | 
|------------------------------------------|------|-------|
| v0.1.0-alpha                             | yes  | yes    |
| master branch                            | no   | yes    |

### Driver parameters
Please refer to [`blobfuse.csi.azure.com` driver parameters](./docs/driver-parameters.md)
 > storage class `blobfuse.csi.azure.com` parameters are compatible with built-in [blobfuse](https://kubernetes.io/docs/concepts/storage/volumes/#blobfuse) plugin

### Prerequisite
 - The driver initialization depends on a [Cloud provider config file](https://github.com/kubernetes/cloud-provider-azure/blob/master/docs/cloud-provider-config.md), usually it's `/etc/kubernetes/azure.json` on all k8s nodes deployed by AKS or aks-engine, here is an [azure.json example](./deploy/example/azure.json)

### Install blobfuse CSI driver on a kubernetes cluster
Please refer to [install blobfuse csi driver](https://github.com/csi-driver/blobfuse-csi-driver/blob/master/docs/install-blobfuse-csi-driver.md)

### E2E Usage example
#### 1. create a pod with csi blobfuse driver mount on linux
##### Option#1: Azurefile Dynamic Provisioning
 - Create an blobfuse CSI storage class
```
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/storageclass-blobfuse-csi.yaml
```

 - Create an blobfuse CSI PVC
```
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pvc-blobfuse-csi.yaml
```

##### Option#2: Azurefile Static Provisioning(use an existing blobfuse share)
 - Use `kubectl create secret` to create `azure-secret` with existing storage account name and key
```
kubectl create secret generic azure-secret --from-literal accountname=NAME --from-literal accountkey="KEY" --type=Opaque
```

 - Create an blobfuse CSI PV, download `pv-blobfuse-csi.yaml` file and edit `sharename` in `volumeAttributes`
```
wget https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pv-blobfuse-csi.yaml
vi pv-blobfuse-csi.yaml
kubectl create -f pv-blobfuse-csi.yaml
```

 - Create an blobfuse CSI PVC which would be bound to the above PV
```
kubectl create -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pvc-blobfuse-csi-static.yaml
```

#### 2. validate PVC status and create an nginx pod
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
```
$ watch kubectl describe po nginx-blobfuse
$ kubectl exec -it nginx-blobfuse -- bash
root@nginx-blobfuse:/# df -h
Filesystem                                                                                             Size  Used Avail Use% Mounted on
overlay                                                                                                 30G   19G   11G  65% /
tmpfs                                                                                                  3.5G     0  3.5G   0% /dev
...
//f571xxx.file.core.windows.net/pvc-file-dynamic-e2ade9f3-f88b-11e8-8429-000d3a03e7d7  1.0G   64K  1.0G   1% /mnt/blobfuse
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
