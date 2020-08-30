## CSI driver example
> refer to [detailed driver parameters](../../docs/driver-parameters.md)

### Dynamic Provisioning (create storage account and blob container by CSI driver)
 - Create CSI storage class
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blobfuse.yaml
```

 - Create a statefulset with blob storage mount
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/statefulset.yaml
```

 - Execute `df -h` command in the container
```
# kubectl exec -it statefulset-blob-0 sh
# df -h
Filesystem      Size  Used Avail Use% Mounted on
...
blobfuse         14G   41M   13G   1% /mnt/blob
...
```

### Static Provisioning(use an existing storage account)
#### Option#1: Use storage class
> make sure cluster identity could access storage account
 - Download [blob storage CSI storage class](https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blobfuse-existing-container.yaml), edit `resourceGroup`, `storageAccount`, `containerName` in storage class
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: blob
provisioner: blob.csi.azure.com
parameters:
  resourceGroup: EXISTING_RESOURCE_GROUP_NAME
  storageAccount: EXISTING_STORAGE_ACCOUNT_NAME  # cross subscription is not supported
  containerName: EXISTING_CONTAINER_NAME
reclaimPolicy: Retain  # If set as "Delete" container would be removed after pvc deletion
volumeBindingMode: Immediate
```

 - Create storage class and PVC
```console
kubectl create -f storageclass-blobfuse-existing-container.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/pvc-blob-csi.yaml
```

#### Option#2: Use secret
 - Use `kubectl create secret` to create `azure-secret` with existing storage account name and key(or sastoken)
```console
kubectl create secret generic azure-secret --from-literal azurestorageaccountname=NAME --from-literal azurestorageaccountkey="KEY" --type=Opaque
#kubectl create secret generic azure-secret --from-literal azurestorageaccountname=NAME --from-literal azurestorageaccountsastoken
="sastoken" --type=Opaque
```

> storage account key(or sastoken) could also be stored in Azure Key Vault, check example here: [read-from-keyvault](../../docs/read-from-keyvault.md)

 - Create a blob storage CSI PV: download [`pv-blobfuse-csi.yaml` file](https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/pv-blobfuse-csi.yaml) and edit `containerName` in `volumeAttributes`
```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: pv-blob
spec:
  capacity:
    storage: 10Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain  # "Delete" is not supported in static provisioning
  csi:
    driver: blob.csi.azure.com
    readOnly: false
    volumeHandle: uniqe-volumeid  # make sure this volumeid is unique in the cluster
    volumeAttributes:
      containerName: EXISTING_CONTAINER_NAME
    nodeStageSecretRef:
      name: azure-secret
      namespace: default
```

 - Create PV and PVC
```console
kubectl create -f pv-blobfuse-csi.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/pvc-blob-csi-static.yaml
```

 - make sure pvc is created and in `Bound` status after a while
```console
kubectl describe pvc pvc-blob
```

#### create a pod with PVC mount
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/nginx-pod-blob.yaml
```

 - Execute `df -h` command in the container
```console
$ kubectl exec -it nginx-blob -- bash
Filesystem      Size  Used Avail Use% Mounted on
...
blobfuse         14G   41M   13G   1% /mnt/blob
...
```
In the above example, there is a `/mnt/blob` directory mounted as `blobfuse` filesystem.
