# Mount Azure blob storage with managed identity

This article demonstrates the process of utilizing blobfuse mount with either a dedicated user-assigned managed identity or kubelet identity.
> make sure the managed identity used by CSI driver is bound to the agent node pool.

## Before you begin
 - Make sure the managed identity has `Storage Blob Data Owner` role to the storage account
 > here is an example that uses Azure CLI commands to assign the `Storage Blob Data Owner` role to the managed identity for the storage account. If the storage account is created by the driver(dynamic provisioning), then you need to grant `Storage Blob Data Owner` role to the resource group where the storage account is located

 > If you use parameter `allowSharedKeyAccess: false` in storageclass, you should assign the `Storage Blob Data Contributor` role to the managed identity for the storage account.

```bash
mid="$(az identity list -g "$resourcegroup" --query "[?name == 'managedIdentityName'].principalId" -o tsv)"
said="$(az storage account list -g "$resourcegroup" --query "[?name == '$storageaccountname'].id" -o tsv)"
az role assignment create --assignee-object-id "$mid" --role "Storage Blob Data Owner" --scope "$said"
```

 - Retrieve the clientID for `AzureStorageIdentityClientID`. If you are using kubelet identity, the identity will be named {aks-cluster-name}-agentpool and located in the node resource group.
```bash
AzureStorageIdentityClientID=`az identity list -g "$resourcegroup" --query "[?name == '$identityname'].clientId" -o tsv`
```
    
## Dynamic Provisioning
- Ensure that the system-assigned identity of your cluster control plane has the `Storage Account Contributor role` for the storage account.
 > if the storage account is created by the driver, then you need to grant `Storage Account Contributor` role to the resource group where the storage account is located

 > AKS cluster control plane identity already has `Contributor` role on the node resource group by default.

1. Create a storage class
    ```yml
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: blob-fuse
    provisioner: blob.csi.azure.com
    parameters:
      skuName: Premium_LRS 
      protocol: fuse
      resourceGroup: EXISTING_RESOURCE_GROUP_NAME   # optional, node resource group by default if it's not provided
      storageAccount: EXISTING_STORAGE_ACCOUNT_NAME # optional, a new account will be created if it's not provided
      containerName: EXISTING_CONTAINER_NAME  # optional, a new container will be created if it's not provided
      AzureStorageAuthType: MSI
      AzureStorageIdentityClientID: "xxxxx-xxxx-xxx-xxx-xxxxxxx"
    reclaimPolicy: Delete
    volumeBindingMode: Immediate
    allowVolumeExpansion: true
    mountOptions:
      - -o allow_other
      - --file-cache-timeout-in-seconds=120
      - --use-attr-cache=true
      - --cancel-list-on-mount-seconds=10  # prevent billing charges on mounting
      - -o attr_timeout=120
      - -o entry_timeout=120
      - -o negative_timeout=120
      - --log-level=LOG_WARNING  # LOG_WARNING, LOG_INFO, LOG_DEBUG
      - --cache-size-mb=1000  # Default will be 80% of available memory, eviction will happen beyond that.
    ```

1. create a statefulset with blobfuse volume mount
```bash
kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/statefulset.yaml
```

## Static Provisioning

> bring your own storage account and blob container

1. create PV with specified account name, blob container and AzureStorageIdentityClientID
    ```yml
    apiVersion: v1
    kind: PersistentVolume
    metadata:
      name: pv-blob
    spec:
      capacity:
        storage: 100Gi
      accessModes:
        - ReadWriteMany
      persistentVolumeReclaimPolicy: Retain  # If set as "Delete" container would be removed after pvc deletion
      storageClassName: blob-fuse
      mountOptions:
        - -o allow_other
        - --file-cache-timeout-in-seconds=120
      csi:
        driver: blob.csi.azure.com
        # make sure this volumeid is unique in the cluster
        # `#` is not allowed in self defined volumeHandle
        volumeHandle: pv-blob
        volumeAttributes:
          protocol: fuse
          resourceGroup: EXISTING_RESOURCE_GROUP_NAME   # optional, node resource group if it's not provided
          storageAccount: EXISTING_STORAGE_ACCOUNT_NAME
          containerName: EXISTING_CONTAINER_NAME
          AzureStorageAuthType: MSI
          AzureStorageIdentityClientID: "xxxxx-xxxx-xxx-xxx-xxxxxxx"
    ```

1. create a pvc and a deployment with blobfuse volume mount
    ```console
    kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/deployment.yaml
    ```
