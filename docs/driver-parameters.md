## Driver Parameters
 > parameter names are case-insensitive

### Dynamic Provisioning
  > get a [blobfuse example](../deploy/example/storageclass-blobfuse-csi.yaml)
 
  > get a `mountOptions` [example](../deploy/example/storageclass-blobfuse-csi-mountoptions.yaml)
 
  > get a [nfs example](../deploy/example/storageclass-blob-nfs.yaml)

Name | Meaning | Example | Mandatory | Default value
--- | --- | --- | --- | ---
skuName | Azure storage account type (alias: `storageAccountType`) | `Standard_LRS`, `Premium_LRS`, `Standard_GRS`, `Standard_RAGRS` | No | `Standard_LRS`
location | Azure location | `eastus`, `westus`, etc. | No | if empty, driver will use the same location name as current k8s cluster
resourceGroup | Azure resource group name | existing resource group name | No | if empty, driver will use the same resource group name as current k8s cluster
storageAccount | specify Azure storage account name| STORAGE_ACCOUNT_NAME | - No for blobfuse mount </br> - Yes for NFSv3 mount |  - For blobfuse mount: if empty, driver will find a suitable storage account that matches `skuName` in the same resource group; if a storage account name is provided, storage account must exist. </br>  - For NFSv3 mount, storage account name must be provided
protocol | specify blobfuse mount or NFSv3 mount | `fuse`, `nfs` | No | `fuse`
containerName | specify the existing container name | existing container name | No | if empty, driver will create a new container name, starting with `pvc-fuse`
server | specify Azure storage account server address | existing server address, e.g. `accountname.privatelink.blob.core.windows.net` | No | if empty, driver will use default `accountname.blob.core.windows.net` or other sovereign cloud account address

 - `fsGroup` securityContext setting

Blobfuse driver does not honor `fsGroup` securityContext setting, instead user could use `-o gid=1000` in `mountoptions` to set ownership, check [here](https://github.com/Azure/Azure-storage-fuse#mount-options) for more mountoptions.

### Static Provisioning(bring your own storage container)
  > get an [example](../deploy/example/pv-blobfuse-csi.yaml)
  >
  > get a key vault [example](../deploy/example/keyvault/pv-blobfuse-csi-keyvault.yaml)

Name | Meaning | Available Value | Mandatory | Default value
--- | --- | --- | --- | ---
volumeAttributes.containerName | existing container name | existing container name | Yes |
volumeAttributes.storageAccountName | existing storage account name | existing storage account name | Yes |
volumeAttributes.protocol | specify blobfuse mount or NFSv3 mount | `fuse`, `nfs` | No | `fuse`
volumeAttributes.keyVaultURL | Azure Key Vault DNS name | existing Azure Key Vault DNS name | No |
volumeAttributes.keyVaultSecretName | Azure Key Vault secret name | existing Azure Key Vault secret name | No |
volumeAttributes.keyVaultSecretVersion | Azure Key Vault secret version | existing version | No |if empty, driver will use "current version"
nodeStageSecretRef.name | secret name that stores storage account name and key(or sastoken) | existing Kubernetes secret name |  No  |
nodeStageSecretRef.namespace | namespace where the secret is | k8s namespace  |  Yes  |
