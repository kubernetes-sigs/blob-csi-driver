## `blobfuse.csi.azure.com` driver parameters
 > storage class `blobfuse.csi.azure.com` parameters are compatible with built-in [blobfuse](https://kubernetes.io/docs/concepts/storage/volumes/#blobfuse) plugin
 
 > parameter names are case-insensitive

 - Dynamic Provisioning
  > get a quick example [here](../deploy/example/storageclass-blobfuse-csi.yaml)

Name | Meaning | Example | Mandatory | Default value 
--- | --- | --- | --- | ---
skuName | blobfuse storage account type (alias: `storageAccountType`) | `Standard_LRS`, `Standard_GRS`, `Standard_RAGRS` | No | `Standard_LRS`
location | specify the location in which blobfuse share will be created | `eastus`, `westus`, etc. | No | if empty, driver will use the same location name as current k8s cluster
resourceGroup | specify the existing resource group name where the container is | existing resource group name | No | if empty, driver will use the same resource group name as current k8s cluster
storageAccount | specify the storage account name in which blobfuse share will be created | STORAGE_ACCOUNT_NAME | No | if empty, driver will find a suitable storage account that matches `skuName` in the same resource group; if a storage account name is provided, it means that storage account must exist otherwise there would be error
containerName | specify the existing container name where blob storage will be created | existing container name | No | if empty, driver will create a new container name, starting with `pvc-fuse`

 - Static Provisioning(use existing storage container)
  > get a quick example [here](../deploy/example/pv-blobfuse-csi.yaml)

Name | Meaning | Available Value | Mandatory | Default value
--- | --- | --- | --- | ---
volumeAttributes.containerName | existing container name | existing container name | Yes |
nodePublishSecretRef.name | secret name that stores storage account name and key | existing secret name |  Yes  | 
nodePublishSecretRef.namespace | namespace where the secret is | k8s namespace  |  No  | `default`
