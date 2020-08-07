## CSI driver E2E usage example
create a pod with blobfuse mount on linux
### Dynamic Provisioning (create storage account and container by Blob Storage CSI driver)
 - Create a blobfuse CSI storage class
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/master/deploy/example/storageclass-blobfuse-csi.yaml
```

 - Create a blobfuse CSI PVC
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/master/deploy/example/pvc-blob-csi.yaml
```

### Static Provisioning(use an existing storage account)
#### Option#1: use existing credentials in k8s cluster
 > make sure the existing credentials in k8s cluster(e.g. service principal, msi) could access the specified storage account
 - Download [blobfuse CSI storage class](https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/master/deploy/example/storageclass-blobfuse-csi-existing-container.yaml), edit `resourceGroup`, `storageAccount`, `containerName` in storage class
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: blob
provisioner: blob.csi.azure.com
parameters:
  skuName: Standard_LRS  # available values: Standard_LRS, Standard_GRS, Standard_RAGRS
  resourceGroup: EXISTING_RESOURCE_GROUP
  storageAccount: EXISTING_STORAGE_ACCOUNT
  containerName: EXISTING_CONTAINER_NAME
reclaimPolicy: Retain  # if set as "Delete" container would be removed after pvc deletion
volumeBindingMode: Immediate
```
```console
kubectl create -f storageclass-blobfuse-csi-existing-container.yaml
```

 - Create a blobfuse CSI PVC
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/master/deploy/example/pvc-blob-csi.yaml
```

#### Option#2: provide storage account name and key(or sastoken)
 - Use `kubectl create secret` to create `azure-secret` with existing storage account name and key(or sastoken)
```console
kubectl create secret generic azure-secret --from-literal azurestorageaccountname=NAME --from-literal azurestorageaccountkey="KEY" --type=Opaque
#kubectl create secret generic azure-secret --from-literal azurestorageaccountname=NAME --from-literal azurestorageaccountsastoken
="sastoken" --type=Opaque
```

> storage account key(or sastoken) could also be stored in Azure Key Vault, check example here: [read-from-keyvault](../../docs/read-from-keyvault.md)

 - Create a blobfuse CSI PV: download [`pv-blobfuse-csi.yaml` file](https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/master/deploy/example/pv-blobfuse-csi.yaml) and edit `containerName` in `volumeAttributes`
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
    volumeHandle: arbitrary-volumeid
    volumeAttributes:
      containerName: EXISTING_CONTAINER_NAME
    nodeStageSecretRef:
      name: azure-secret
      namespace: default
```
```console
kubectl create -f pv-blobfuse-csi.yaml
```

 - Create a blobfuse CSI PVC which would be bound to the above PV
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/master/deploy/example/pvc-blob-csi-static.yaml
```

#### 2. Validate PVC status and create an nginx pod
 > make sure pvc is created and in `Bound` status
```console
watch kubectl describe pvc pvc-blob
```

 - create a pod with blobfuse CSI PVC
```console
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blobfuse-csi-driver/master/deploy/example/nginx-pod-blob.yaml
```

#### 3. enter the pod container to do validation
 - watch the status of pod until its Status changed from `Pending` to `Running` and then enter the pod container
```console
$ watch kubectl describe po nginx-blob
$ kubectl exec -it nginx-blob -- bash
Filesystem      Size  Used Avail Use% Mounted on
...
blobfuse         30G  8.9G   21G  31% /mnt/blob
/dev/sda1        30G  8.9G   21G  31% /etc/hosts
...
```
In the above example, there is a `/mnt/blob` directory mounted as `blobfuse` filesystem.
