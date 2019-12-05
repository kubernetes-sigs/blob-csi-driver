## CSI driver E2E usage example
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

#### Option#2: provide storage account name and key(or sastoken)
 - Use `kubectl create secret` to create `azure-secret` with existing storage account name and key(or sastoken)
```
kubectl create secret generic azure-secret --from-literal azurestorageaccountname=NAME --from-literal azurestorageaccountkey="KEY" --type=Opaque
#kubectl create secret generic azure-secret --from-literal azurestorageaccountname=NAME --from-literal azurestorageaccountsastoken
="sastoken" --type=Opaque
```

> storage account key(or sastoken) could also be stored in Azure Key Vault, check example here: [read-from-keyvault](./docs/read-from-keyvault.md)

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
In the above example, there is a `/mnt/blobfuse` directory mounted as `blobfuse` filesystem.
