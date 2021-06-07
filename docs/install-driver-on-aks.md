## Set up CSI driver on AKS cluster

 - Prerequisites

AKS cluster is created with user asissnged identity by default, make sure cluster identity has `Contributor` role on node resource group, follow below instruction to set up `Contributor` role on node resource group
![image](https://user-images.githubusercontent.com/4178417/120978367-f68f0a00-c7a6-11eb-8e87-89247d1ddc0b.png):


 - Install CSI driver

install latest **released** CSI driver version, following guide [here](./install-blob-csi-driver.md)

 - Set up new storage classes
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blobfuse.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blob-nfs.yaml
```
