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
storageAccount | specify Azure storage account name| STORAGE_ACCOUNT_NAME | No | If the driver is not provided with a specific storage account name, it will search for a suitable storage account that matches the account settings within the same resource group. If it cannot find a matching storage account, it will create a new one. However, if a storage account name is specified, the storage account must already exist.
protocol | specify blobfuse, blobfuse2 or NFSv3 mount (blobfuse2 is still in Preview) | `fuse`, `fuse2`, `nfs` | No | `fuse`
networkEndpointType | specify network endpoint type for the storage account created by driver. If `privateEndpoint` is specified, a private endpoint will be created for the storage account. For other cases, a service endpoint will be created for NFS protocol. | "",`privateEndpoint` | No | ``<br>for AKS cluster, make sure cluster Control plane identity (that is, your AKS cluster name) is added to the Contributor role in the resource group hosting the VNet
storageEndpointSuffix | specify Azure storage endpoint suffix | `core.windows.net`, `core.chinacloudapi.cn`, etc | No | if empty, driver will use default storage endpoint suffix according to cloud environment, e.g. `core.windows.net`
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
softDeleteBlobs | Enable [soft delete for blobs](https://learn.microsoft.com/en-us/azure/storage/blobs/soft-delete-blob-overview), specify the days to retain deleted blobs | "7" | No | Soft Delete Blobs is disabled if empty
softDeleteContainers | Enable [soft delete for containers](https://learn.microsoft.com/en-us/azure/storage/blobs/soft-delete-container-overview), specify the days to retain deleted containers | "7" | No | Soft Delete Containers is disabled if empty
enableBlobVersioning | Enable [blob versioning](https://learn.microsoft.com/en-us/azure/storage/blobs/versioning-overview), can't enabled when `protocol` is `nfs` or `isHnsEnabled` is `true` | `true`,`false` | No | versioning for blobs is disabled if empty

 - `fsGroup` securityContext setting

Blobfuse driver does not honor `fsGroup` securityContext setting, instead user could use `-o gid=1000` in `mountoptions` to set ownership, check [here](https://github.com/Azure/azure-storage-fuse/tree/blobfuse-1.4.5#mount-options) for more mountoptions.

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
volumeHandle | Specify a value the driver can use to uniquely identify the storage blob container in the cluster. | A recommended way to produce a unique value is to combine the globally unique storage account name and container name: {account-name}_{container-name}. Note: the # character is reserved for internal use. | Yes |
volumeAttributes.resourceGroup | Azure resource group name | existing resource group name | No | if empty, driver will use the same resource group name as current k8s cluster
volumeAttributes.storageAccount | existing storage account name | existing storage account name | Yes |
volumeAttributes.containerName | existing container name | existing container name | Yes |
volumeAttributes.protocol | specify blobfuse, blobfuse2 or NFSv3 mount (blobfuse2 is still in Preview) | `fuse`, `fuse2`, `nfs` | No | `fuse`
--- | **Following parameters are only for blobfuse** | --- | --- |
volumeAttributes.secretName | secret name that stores storage account name and key(only applies for SMB) | | No |
volumeAttributes.secretNamespace | secret namespace | `default`,`kube-system`, etc | No | pvc namespace
nodeStageSecretRef.name | secret name that stores(check below examples):<br>`azurestorageaccountkey`<br>`azurestorageaccountsastoken`<br>`msisecret`<br>`azurestoragespnclientsecret` | existing Kubernetes secret name |  No  |
nodeStageSecretRef.namespace | secret namespace | k8s namespace  |  Yes  |
--- | **Following parameters are only for NFS protocol** | --- | --- |
volumeAttributes.mountPermissions | mounted folder permissions | `0777` | No |
--- | **Following parameters are only for feature: blobfuse [Managed Identity and Service Principal Name auth](https://github.com/Azure/azure-storage-fuse/tree/blobfuse-1.4.5#environment-variables)** | --- | --- |
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
 - mounting blobfuse requires account key, if `nodeStageSecretRef` field is not provided in PV config, azure file driver would try to get `azure-storage-account-{accountname}-secret` in the pod namespace first, if that secret does not exist, it would get account key by Azure storage account API directly using kubelet identity (make sure kubelet identity has reader access to the storage account).
 - mounting blob storage NFSv3 does not need account key, NFS mount access is configured by following setting:
    - `Firewalls and virtual networks`: select `Enabled from selected virtual networks and IP addresses` with same vnet as agent node
 - blobfuse cache(`--tmp-path` [mount option](https://github.com/Azure/azure-storage-fuse/tree/blobfuse-1.4.5#mount-options))
   - blobfuse cache is on `/mnt` directory by default, `/mnt` is mounted on temp disk if VM sku provides temp disk, `/mnt` is mounted on os disk if VM sku does not provide temp disk
   - with blobfuse-proxy deployment (default on AKS), user could set `--tmp-path=` mount option to specify a different cache directory
 - [Mount an azure blob storage with a dedicated user-assigned managed identity](https://github.com/qxsch/Azure-Aks/tree/master/aks-blobfuse-mi)
 - [Blobfuse Performance and caching](https://github.com/Azure/azure-storage-fuse/tree/blobfuse-1.4.5#performance-and-caching)
 - [Blobfuse CLI Flag Options v1 & v2](https://github.com/Azure/azure-storage-fuse/blob/main/MIGRATION.md#blobfuse-cli-flag-options)

#### `containerName` parameter supports following pv/pvc metadata conversion
> if `containerName` value contains following strings, it would be converted into corresponding pv/pvc name or namespace
 - `${pvc.metadata.name}`
 - `${pvc.metadata.namespace}`
 - `${pv.metadata.name}`

#### [Storage considerations for Azure Kubernetes Service (AKS)](https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/scenarios/app-platform/aks/storage)
#### [Compare access to Azure Files, Blob Storage, and Azure NetApp Files with NFS](https://learn.microsoft.com/en-us/azure/storage/common/nfs-comparison#comparison)
