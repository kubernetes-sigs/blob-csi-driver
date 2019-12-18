# Read storage account key(or sastoken) from Azure Key Vault

## Prerequisite

1. Create an Azure Key Vault

2. Store `storage account key` or `SAS token` as `secret` in Azure Key Vault.

3. Ensure service principal in cluster has all the required permissions to access content in your Azure key vault instance. If not, run the following commands:

   ```console
   # Assign Reader Role to the service principal for your keyvault
   aadclientid=
   keyvaultname=

   az role assignment create --role Reader --assignee $aadclientid --scope /subscriptions/<subscriptionid>/resourcegroups/<resourcegroup>/providers/Microsoft.KeyVault/vaults/$keyvaultname

   az keyvault set-policy -n $keyvaultname --key-permissions get --spn $aadclientid
   az keyvault set-policy -n $keyvaultname --secret-permissions get --spn $aadclientid
   az keyvault set-policy -n $keyvaultname --certificate-permissions get --spn $aadclientid
   ```

## Install blobfuse CSI driver on a kubernetes cluster
Please refer to [install blobfuse csi driver](https://github.com/kubernetes-sigs/blobfuse-csi-driver/blob/master/docs/install-blobfuse-csi-driver.md)

## Create PV
1.  Download a `pv-blobfuse-csi-keyvault.yaml`, edit `keyVaultURL`, `keyVaultSecretName`, `containerName` in PV
> `keyVaultSecretVersion` is the optional parameter. If not specified, it will be *current version*.
```console
wget https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pv-blobfuse-csi-keyvault.yaml
vi pv-blobfuse-csi-keyvault.yaml
kubectl apply -f pv-blobfuse-csi-keyvault.yaml
```

## Create PVC 

```console
kubectl apply -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/pvc-blobfuse-csi-static.yaml
```
