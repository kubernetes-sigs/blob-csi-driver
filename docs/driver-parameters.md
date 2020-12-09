## Driver Parameters
 > parameter names are case-insensitive

### Dynamic Provisioning
  > [blobfuse example](../deploy/example/storageclass-blobfuse.yaml)
 
  > [blobfuse mountOptions example](../deploy/example/storageclass-blobfuse-mountoptions.yaml)

  > [nfs example](../deploy/example/storageclass-blob-nfs.yaml)

Name | Meaning | Example | Mandatory | Default value
--- | --- | --- | --- | ---
skuName | Azure storage account type (alias: `storageAccountType`) | `Standard_LRS`, `Premium_LRS`, `Standard_GRS`, `Standard_RAGRS` | No | `Standard_LRS`
location | Azure location | `eastus`, `westus`, etc. | No | if empty, driver will use the same location name as current k8s cluster
resourceGroup | Azure resource group name | existing resource group name | No | if empty, driver will use the same resource group name as current k8s cluster
storageAccount | specify Azure storage account name| STORAGE_ACCOUNT_NAME | - No for blobfuse mount </br> - Yes for NFSv3 mount |  - For blobfuse mount: if empty, driver will find a suitable storage account that matches `skuName` in the same resource group; if a storage account name is provided, storage account must exist. </br>  - For NFSv3 mount, storage account name must be provided
protocol | specify blobfuse mount or NFSv3 mount | `fuse`, `nfs` | No | `fuse`
containerName | specify the existing container name | existing container name | No | if empty, driver will create a new container name, starting with `pvc-fuse` for blobfuse or `pvc-nfs` for NFSv3
server | specify Azure storage account server address | existing server address, e.g. `accountname.privatelink.blob.core.windows.net` | No | if empty, driver will use default `accountname.blob.core.windows.net` or other sovereign cloud account address
tags | [tags](https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/tag-resources) would be created in newly created storage account | tag format: 'foo=aaa,bar=bbb' | No | ""

 - `fsGroup` securityContext setting

Blobfuse driver does not honor `fsGroup` securityContext setting, instead user could use `-o gid=1000` in `mountoptions` to set ownership, check [here](https://github.com/Azure/Azure-storage-fuse#mount-options) for more mountoptions.

### Static Provisioning(bring your own storage container)
  > [blobfuse example](../deploy/example/pv-blobfuse-csi.yaml)

  > [nfs example](../deploy/example/pv-blobfuse-nfs.yaml)

  > [blobfuse read account key or SAS token from key vault example](../deploy/example/keyvault/pv-blobfuse-csi-keyvault.yaml)

  > [blobfuse Managed Identity and Service Principal Name auth example](../deploy/example/pv-blobfuse-auth.yaml)

Name | Meaning | Available Value | Mandatory | Default value
--- | --- | --- | --- | ---
volumeAttributes.resourceGroup | Azure resource group name | existing resource group name | No | if empty, driver will use the same resource group name as current k8s cluster
volumeAttributes.storageAccount | existing storage account name | existing storage account name | Yes |
volumeAttributes.containerName | existing container name | existing container name | Yes |
volumeAttributes.protocol | specify blobfuse mount or NFSv3 mount | `fuse`, `nfs` | No | `fuse`
nodeStageSecretRef.name | secret name that stores(check below examples):<br>`azurestorageaccountkey`<br>`azurestorageaccountsastoken`<br>`msisecret`<br>`azurestoragespnclientsecret` | existing Kubernetes secret name |  No  |
nodeStageSecretRef.namespace | namespace where the secret is | k8s namespace  |  Yes  |
--- | **Following parameters are only for feature: blobfuse [Managed Identity and Service Principal Name auth](https://github.com/Azure/azure-storage-fuse#environment-variables)** | --- | --- |
volumeAttributes.AzureStorageAuthType | Authentication Type | `Key`, `SAS`, `MSI`, `SPN` | No | `Key`
volumeAttributes.AzureStorageIdentityClientID | Identity Client ID |  | No |
volumeAttributes.AzureStorageIdentityObjectID | Identity Object ID |  | No |
volumeAttributes.AzureStorageIdentityResourceID | Identity Resource ID |  | No |
volumeAttributes.MSIEndpoint | MSI Endpoint |  | No |
volumeAttributes.AzureStorageSPNClientID | SPN Client ID |  | No |
volumeAttributes.AzureStorageSPNTenantID | SPN Tenant ID |  | No |
volumeAttributes.AzureStorageAADEndpoint | AADEndpoint |  | No |
--- | **Following parameters are only for feature: blobfuse read account key or SAS token from key vault** | --- | --- |
volumeAttributes.keyVaultURL | Azure Key Vault DNS name | existing Azure Key Vault DNS name | No |
volumeAttributes.keyVaultSecretName | Azure Key Vault secret name | existing Azure Key Vault secret name | No |
volumeAttributes.keyVaultSecretVersion | Azure Key Vault secret version | existing version | No |if empty, driver will use "current version"


 - create a Kubernetes secret for `nodeStageSecretRef.name`
 ```console
kubectl create secret generic azure-secret --from-literal accountname="xxx" accountkey="xxx" --type=Opaque
kubectl create secret generic azure-secret --from-literal accountname="xxx" azurestorageaccountsastoken="xxx" --type=Opaque
kubectl create secret generic azure-secret --from-literal msisecret="xxx" --type=Opaque
kubectl create secret generic azure-secret --from-literal azurestoragespnclientsecret="xxx" --type=Opaque
 ```
