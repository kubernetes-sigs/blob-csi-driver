## NFSv3 support
[NFS 3.0 protocol support on Azure Blob storage](https://docs.microsoft.com/en-us/azure/storage/blobs/network-file-system-protocol-support) is now in Preview. This service is best suited for large scale read-heavy sequential access workload where data will be ingested once and minimally modified further. E.g. large scale analytic data, backup and archive, NFS apps for media rendering, and genomic sequencing etc. It offers lowest total cost of ownership.

#### Feature Status: Alpha
> supported OS: Linux

#### Available regions
`eastus`, `centralus`, `canadacentral`

#### Prerequisite
 - [Register the NFS 3.0 protocol feature with your subscription](https://docs.microsoft.com/en-us/azure/storage/blobs/network-file-system-protocol-support-how-to#step-1-register-the-nfs-30-protocol-feature-with-your-subscription)
```console
az feature register --name AllowNFSV3 --namespace Microsoft.Storage
az feature register --name PremiumHns --namespace Microsoft.Storage
az provider register --namespace Microsoft.Storage
```

 - [install CSI driver](https://github.com/kubernetes-sigs/blobfuse-csi-driver/blob/master/docs/install-csi-driver-master.md) (only master version supported now)
 - Create a `Premium_LRS` Azure storage account with following configurations to support NFS 3.0
   - account kind: `BlockBlobStorage`
   - Replication: `Locally-redundant storage (LRS)`
   - secure transfer required(enable HTTPS traffic only): `false`
   - select virtual network of agent nodes in `Firewalls and virtual networks`
   - Hierarchical namespace: `Enabled`
   - NFS V3: `Enabled`

#### How to use NFS feature
 - Create an Azure File storage class
> specify `storageAccount` and `protocol: nfs` in storage class `parameters`
> </br>for more details, refer to [driver parameters](../../../docs/driver-parameters.md)
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: blob
provisioner: blobfuse.csi.azure.com
parameters:
  resourceGroup: EXISTING_RESOURCE_GROUP_NAME  # optional, only set this when storage account is not in the same resource group as agent node
  storageAccount: EXISTING_STORAGE_ACCOUNT_NAME
  protocol: nfs
```

run following command to create a storage class:
```console
wget https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/master/deploy/example/storageclass-blob-nfs.yaml
# set `storageAccount` in storageclass-blob-nfs.yaml
kubectl create -f storageclass-blob-nfs.yaml
```

### Example
 - Create a deployment with NFSv3 on Azure storage
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/master/deploy/example/statefulset.yaml
```

 - enter pod to check
```console
$ exec -it statefulset-blobfuse-0 bash
# df -h
Filesystem      Size  Used Avail Use% Mounted on
...
/dev/sda1                                                                                 29G   11G   19G  37% /etc/hosts
accountname.blob.core.windows.net:/accountname/pvc-cce02240-5d13-4bcb-b9eb-f9c7eeaaa640  256T     0  256T   0% /mnt/blobfuse
...
```
