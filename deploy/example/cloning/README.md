# Volume Cloning Example
## Feature Status: Beta

- supported from v1.23.2
- NFSv3 protocol is not supported

## Prerequisites
- Make sure that the virtual network hosting the driver controller pod is added to the list of allowed virtual networks in the storage account's VNet settings
  - if the driver controller pod is managed by AKS, you need to set `Enable from all networks` in the storage account's VNet settings
- Before proceeding, ensure that the application is not writing data to the source volume.
- In order to allow the use of a managed identity, the source storage account requires the `Storage Blob Data Reader` role, while the destination storage account requires the `Storage Blob Data Contributor` role. If the Storage Blob Data role is not granted, the CSI driver will use SAS token as fallback.

## Create a Source PVC

```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blobfuse2.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/pvc-blob-csi.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/nginx-pod-blob.yaml
```

### Check the Source PVC

```console
$ kubectl exec nginx-blob -- ls /mnt/blob
outfile
```

## Create a PVC from an existing PVC
>  Make sure application is not writing data to source blob container
```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/cloning/pvc-blob-csi-cloning.yaml
```
### Check the Creation Status

```console
$ kubectl describe pvc pvc-blob-cloning
Name:          pvc-blob-cloning
Namespace:     default
StorageClass:  blob-fuse
Status:        Bound
Volume:        pvc-6db5af93-3b32-4c24-a68e-b727d7801fd5
Labels:        <none>
Annotations:   kubectl.kubernetes.io/last-applied-configuration:
                 {"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"annotations":{},"name":"pvc-blob-cloning","namespace":"default"},"spec":{"a...
               pv.kubernetes.io/bind-completed: yes
               pv.kubernetes.io/bound-by-controller: yes
               volume.beta.kubernetes.io/storage-provisioner: blob.csi.azure.com
               volume.kubernetes.io/storage-provisioner: blob.csi.azure.com
Finalizers:    [kubernetes.io/pvc-protection]
Capacity:      100Gi
Access Modes:  RWX
VolumeMode:    Filesystem
Mounted By:    <none>
Events:
  Type    Reason                 Age                From                                                                                       Message
  ----    ------                 ----               ----                                                                                       -------
  Normal  Provisioning           16s                blob.csi.azure.com_aks-nodepool1-34988195-vmss000002_8ecdf8ad-b636-4ca5-81ee-0f1a49337168  External provisioner is provisioning volume for claim "default/pvc-blob-cloning"
  Normal  ExternalProvisioning   14s (x3 over 16s)  persistentvolume-controller                                                                waiting for a volume to be created, either by external provisioner "blob.csi.azure.com" or manually created by system administrator
  Normal  ProvisioningSucceeded  8s                 blob.csi.azure.com_aks-nodepool1-34988195-vmss000002_8ecdf8ad-b636-4ca5-81ee-0f1a49337168  Successfully provisioned volume pvc-6db5af93-3b32-4c24-a68e-b727d7801fd5
```

## Restore the PVC into a Pod

```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/cloning/nginx-pod-restored-cloning.yaml
```

### Check Sample Data

```console
$ kubectl exec nginx-blob-restored-cloning -- ls /mnt/blob
outfile
```
