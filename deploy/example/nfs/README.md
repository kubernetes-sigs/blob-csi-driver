## NFSv3 support
[NFS 3.0 protocol support on Azure Blob storage](https://docs.microsoft.com/en-us/azure/storage/blobs/network-file-system-protocol-support) is best suited for large scale read-heavy sequential access workload where data will be ingested once and minimally modified further. e.g. large scale analytic data, backup and archive, NFS apps for media rendering, and genomic sequencing etc. It offers lowest total cost of ownership.
 - [Compare access to Azure Files, Blob Storage, and Azure NetApp Files with NFS](https://docs.microsoft.com/en-us/azure/storage/common/nfs-comparison)

#### Supported OS: Linux
 - dynamic account creation support is available from `v1.2.0`

#### Prerequisite
 - Make sure identity used by the driver controller is added to the Contributor role on the virtual network and network security group of the cluster.
 - [Optional][Bring Your Own Storage Account] Follow steps [here](https://docs.microsoft.com/en-us/azure/storage/blobs/network-file-system-protocol-support-how-to) to create storage account that supports NFSv3 protocol and then specify `storageAccount` in below storage class `parameters`

#### How to use NFS feature
 - Create an Azure File storage class
> specify `protocol: nfs` in storage class `parameters`
> </br>for more details, refer to [driver parameters](../../../docs/driver-parameters.md)
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: blob-nfs
provisioner: blob.csi.azure.com
parameters:
  protocol: nfs
```

run following command to create a storage class:
```console
wget https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blob-nfs.yaml
# set `storageAccount` in storageclass-blob-nfs.yaml
kubectl create -f storageclass-blob-nfs.yaml
```

### Example
 - Create a deployment with NFSv3 on Azure storage
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/nfs/statefulset.yaml
```

 - enter pod to check
```console
kubectl exec -it statefulset-blob-0 -- df -h
```
<pre>
Filesystem      Size  Used Avail Use% Mounted on
...
/dev/sda1                                                                                 29G   11G   19G  37% /etc/hosts
accountname.blob.core.windows.net:/accountname/pvc-cce02240-5d13-4bcb-b9eb-f9c7eeaaa640  256T     0  256T   0% /mnt/blob
...
# ls -lt
total 2
-rw-r--r-- 1 root root 1120 Sep  3 06:52 outfile
</pre>
