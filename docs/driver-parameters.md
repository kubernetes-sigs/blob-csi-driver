## Driver Parameters
 > parameter names are case-insensitive

### Dynamic Provisioning
  > [blobfuse example](../deploy/example/storageclass-blobfuse.yaml)

  > [nfs example](../deploy/example/storageclass-blob-nfs.yaml)

Name | Meaning | Example | Mandatory | Default value
--- | --- | --- | --- | ---
skuName | Azure storage account type (alias: `storageAccountType`) | `Standard_LRS`, `Premium_LRS`, `Standard_GRS`, `Standard_RAGRS` | No | `Standard_LRS`
location | Azure location | `eastus`, `westus`, etc. | No | if empty, driver will use the same location name as current k8s cluster
resourceGroup | Azure resource group name | existing resource group name | No | if empty, driver will use the same resource group name as current k8s cluster
storageAccount | specify Azure storage account name| STORAGE_ACCOUNT_NAME | - No for blobfuse mount </br> - Yes for NFSv3 mount |  - For blobfuse mount: if empty, driver will find a suitable storage account that matches `skuName` in the same resource group; if a storage account name is provided, storage account must exist. </br>  - For NFSv3 mount, storage account name must be provided
protocol | specify blobfuse mount or NFSv3 mount | `fuse`, `nfs` | No | `fuse`
containerName | specify the existing container(directory) name | existing container name | No | if empty, driver will create a new container name, starting with `pvc-fuse` for blobfuse or `pvc-nfs` for NFSv3
containerNamePrefix | specify Azure storage directory prefix created by driver | can only contain lowercase letters, numbers, hyphens, and length should be less than 21 | No |
server | specify Azure storage account server address | existing server address, e.g. `accountname.privatelink.blob.core.windows.net` | No | if empty, driver will use default `accountname.blob.core.windows.net` or other sovereign cloud account address
accessTier | [Access tier for storage account](https://learn.microsoft.com/en-us/azure/storage/blobs/access-tiers-overview) | Standard account can choose `Hot` or `Cool`, and Premium account can only choose `Premium` | No | empty(use default setting for different storage account types)
allowBlobPublicAccess | Allow or disallow public access to all blobs or containers for storage account created by driver | `true`,`false` | No | `false`
requireInfraEncryption | specify whether or not the service applies a secondary layer of encryption with platform managed keys for data at rest for storage account created by driver | `true`,`false` | No | `false`
storageEndpointSuffix | specify Azure storage endpoint suffix | `core.windows.net`, `core.chinacloudapi.cn`, etc | No | if empty, driver will use default storage endpoint suffix according to cloud environment
tags | [tags](https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/tag-resources) would be created in newly created storage account | tag format: 'foo=aaa,bar=bbb' | No | ""
matchTags | whether matching tags when driver tries to find a suitable storage account | `true`,`false` | No | `false`
useDataPlaneAPI | specify whether use data plane API for blob container create/delete, this could solve the SRP API throltting issue since data plane API has almost no limit, while it would fail when there is firewall or vnet setting on storage account | `true`,`false` | No | `false`
--- | **Following parameters are only for blobfuse** | --- | --- |
subscriptionID | specify Azure subscription ID in which blob storage directory will be created | Azure subscription ID | No | if not empty, `resourceGroup` must be provided
storeAccountKey | whether store account key to k8s secret <br><br> Note:  <br> `false` means driver would leverage kubelet identity to get account key | `true`,`false` | No | `true`
secretName | specify secret name to store account key | | No |
secretNamespace | specify the namespace of secret to store account key | `default`,`kube-system`, etc | No | pvc namespace
isHnsEnabled | enable `Hierarchical namespace` for Azure DataLake storage account | `true`,`false` | No | `false`
--- | **Following parameters are only for NFS protocol** | --- | --- |
mountPermissions | mounted folder permissions. The default is `0777`, if set as `0`, driver will not perform `chmod` after mount | `0777` | No |
vnetResourceGroup | specify vnet resource group where virtual network is | existing resource group name | No | if empty, driver will use the `vnetResourceGroup` value in azure cloud config file
vnetName | virtual network name | existing virtual network name | No | if empty, driver will use the `vnetName` value in azure cloud config file
subnetName | subnet name | existing subnet name of the agent node | No | if empty, driver will use the `subnetName` value in azure cloud config file

 - `fsGroup` securityContext setting

Blobfuse driver does not honor `fsGroup` securityContext setting, instead user could use `-o gid=1000` in `mountoptions` to set ownership, check [here](https://github.com/Azure/Azure-storage-fuse#mount-options) for more mountoptions.

 - [Azure DataLake storage account](https://docs.microsoft.com/en-us/azure/storage/blobs/upgrade-to-data-lake-storage-gen2-how-to) support
   - set `isHnsEnabled: "true"` in storage class parameter to create ADLS account by driver in dynamic provisioning.
   - mount option `--use-adls=true` must be specified to enable blobfuse access ADLS account in static provisioning.

 - account tags format created by dynamic provisioning
```
k8s-azure-created-by: azure
```

 - file share name format created by dynamic provisioning(example)
```
pvc-92a4d7f2-f23b-4904-bad4-2cbfcff6e388
```

 - VolumeID(`volumeHandle`) is the identifier for the volume handled by the driver, format of VolumeID: `rg#accountName#containerName#uuid#secretNamespace#subscriptionID`
 > `uuid`, `secretNamespace`, `subscriptionID` are optional

### Static Provisioning(bring your own storage container)
  > [blobfuse example](../deploy/example/pv-blobfuse-csi.yaml)

  > [nfs example](../deploy/example/pv-blobfuse-nfs.yaml)

  > [blobfuse read account key or SAS token from key vault example](../deploy/example/pv-blobfuse-csi-keyvault.yaml)

  > [blobfuse Managed Identity and Service Principal Name auth example](../deploy/example/pv-blobfuse-auth.yaml)

Name | Meaning | Available Value | Mandatory | Default value
--- | --- | --- | --- | ---
volumeAttributes.resourceGroup | Azure resource group name | existing resource group name | No | if empty, driver will use the same resource group name as current k8s cluster
volumeAttributes.storageAccount | existing storage account name | existing storage account name | Yes |
volumeAttributes.containerName | existing container name | existing container name | Yes |
volumeAttributes.protocol | specify blobfuse mount or NFSv3 mount | `fuse`, `nfs` | No | `fuse`
--- | **Following parameters are only for blobfuse** | --- | --- |
volumeAttributes.secretName | secret name that stores storage account name and key(only applies for SMB) | | No |
volumeAttributes.secretNamespace | secret namespace | `default`,`kube-system`, etc | No | pvc namespace
nodeStageSecretRef.name | secret name that stores(check below examples):<br>`azurestorageaccountkey`<br>`azurestorageaccountsastoken`<br>`msisecret`<br>`azurestoragespnclientsecret` | existing Kubernetes secret name |  No  |
nodeStageSecretRef.namespace | secret namespace | k8s namespace  |  Yes  |
--- | **Following parameters are only for NFS protocol** | --- | --- |
volumeAttributes.mountPermissions | mounted folder permissions | `0777` | No |
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
volumeAttributes.keyVaultSecretVersion | Azure Key Vault secret version | existing version | No |if empty, driver will use `current version`

 - create a Kubernetes secret for `nodeStageSecretRef.name`
 ```console
kubectl create secret generic azure-secret --from-literal=azurestorageaccountname="xxx" --from-literal azurestorageaccountkey="xxx" --type=Opaque
kubectl create secret generic azure-secret --from-literal=azurestorageaccountname="xxx" --from-literal azurestorageaccountsastoken="xxx" --type=Opaque
kubectl create secret generic azure-secret --from-literal msisecret="xxx" --type=Opaque
kubectl create secret generic azure-secret --from-literal azurestoragespnclientsecret="xxx" --type=Opaque
 ```

### Tips
 - only mounting blobfuse requires account key, and if secret is not provided in PV config, driver would try to get `azure-storage-account-{accountname}-secret` in the pod namespace, if not found, driver would try using kubelet identity to get account key directly using Azure API.
 - mounting blob storage NFSv3 does not need account key, it requires storage account configured with same vnet with agent node.
 - blobfuse does not support private link well, check details [here](https://github.com/Azure/azure-storage-fuse/wiki/2.-Configuring-and-Running#private-link)
 - [Mount an azure blob storage with a dedicated user-assigned managed identity](https://github.com/qxsch/Azure-Aks/tree/master/aks-blobfuse-mi)

#### `containerName` parameter supports following pv/pvc metadata conversion
> if `containerName` value contains following strings, it would be converted into corresponding pv/pvc name or namespace
 - `${pvc.metadata.name}`
 - `${pvc.metadata.namespace}`
 - `${pv.metadata.name}`
