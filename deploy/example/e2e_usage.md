## CSI driver example
> refer to [driver parameters](../../docs/driver-parameters.md) for more detailed usage

### Dynamic Provisioning
#### Option#1: create storage account by CSI driver
 - Create storage class
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blobfuse.yaml
```

#### Option#2: bring your own storage account
 > only available from `v0.9.0`
 > This option does not depend on cloud provider config file, supports cross subscription and on-premise cluster scenario.
 - Use `kubectl create secret` to create `azure-secret` with existing storage account name and key
```console
kubectl create secret generic azure-secret --from-literal azurestorageaccountname=NAME --from-literal azurestorageaccountkey="KEY" --type=Opaque
```

 - create storage class referencing `azure-secret`
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blob-secret.yaml
```

#### Create application
 - Create a statefulset with volume mount
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/statefulset.yaml
```

 - Execute `df -h` command in the container
```console
kubectl exec -it statefulset-blob-0 -- df -h
```
<pre>
Filesystem      Size  Used Avail Use% Mounted on
...
blobfuse         14G   41M   13G   1% /mnt/blob
...
</pre>

### Static Provisioning(use an existing storage account)
#### Option#1: Use storage class
> make sure cluster identity could access storage account
 - Download [blob storage CSI storage class](https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blobfuse-existing-container.yaml), edit `resourceGroup`, `storageAccount`, `containerName` in storage class
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: blob-fuse
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
```

or create `azure-secret` with existing storage account name and sastoken:

```console
kubectl create secret generic azure-secret --from-literal azurestorageaccountname=NAME --from-literal azurestorageaccountsastoken
="sastoken" --type=Opaque
```

> storage account key(or sastoken) could also be stored in Azure Key Vault, check example here: [read-from-keyvault](../../docs/read-from-keyvault.md)

 - Create PV: download [`pv-blobfuse-csi.yaml` file](https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/pv-blobfuse-csi.yaml) and edit `containerName` in `volumeAttributes`
```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  annotations:
    pv.kubernetes.io/provisioned-by: blob.csi.azure.com
  name: pv-blob
spec:
  capacity:
    storage: 10Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  csi:
    driver: blob.csi.azure.com
    readOnly: false
    # make sure volumeid is unique for every storage blob container in the cluster
    # the # character is reserved for internal use
    volumeHandle: account-name_container-name
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
kubectl exec -it nginx-blob -- df -h
```
<pre>
Filesystem      Size  Used Avail Use% Mounted on
...
blobfuse         14G   41M   13G   1% /mnt/blob
...
</pre>

In the above example, there is a `/mnt/blob` directory mounted as `blobfuse` filesystem.

#### Option#3: Inline volume
 > only available from `v1.2.0` for blobfuse protocol (NFS protocol is not supported)
 - Create `azure-secret` with existing storage account name and key in the same namespace as pod
 > in below example, both secret and pod are in `default` namespace
```console
kubectl create secret generic azure-secret --from-literal azurestorageaccountname=NAME --from-literal azurestorageaccountkey="KEY" --type=Opaque
```

 - download `nginx-pod-azurefile-inline-volume.yaml` file and edit `containerName`, `secretName`
```console
wget https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/nginx-blobfuse-inline-volume.yaml
#edit nginx-blobfuse-inline-volume.yaml
kubectl create -f nginx-blobfuse-inline-volume.yaml
```
