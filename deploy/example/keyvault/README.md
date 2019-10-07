# Use Blobfuse CSI Driver with Azure Key Vault

> Attention: Currently, we just support use Key Vault in static provisioning scenario.

## Prepare Key Vault

1. Create an Azure Key Vault

2. Store `storage account key` or `SAS token` as `secret` in Azure Key Vault.

3. Ensure service principal in cluster has all the required permissions to access content in your Azure key vault instance. If not, run the following commands:

   ```console
   # Assign Reader Role to the service principal for your keyvault
   az role assignment create --role Reader --assignee <YOUR SPN CLIENT ID> --scope /subscriptions/<subscriptionid>/resourcegroups/<resourcegroup>/providers/Microsoft.KeyVault/vaults/$keyvaultname
   
   az keyvault set-policy -n $keyvaultname --key-permissions get --spn <YOUR SPN CLIENT ID>
   az keyvault set-policy -n $keyvaultname --secret-permissions get --spn <YOUR SPN CLIENT ID>
   az keyvault set-policy -n $keyvaultname --certificate-permissions get --spn <YOUR CLIENT ID>
   ```

## Install blobfuse CSI driver on a kubernetes cluster
Please refer to [install blobfuse csi driver](https://github.com/csi-driver/blobfuse-csi-driver/blob/master/docs/install-blobfuse-csi-driver.md)

## Create PV
1.  Download a `pv-blobfuse-csi-keyvault.yaml`, edit `keyVaultURL`, `keyVaultSecretName`, `containerName` in PV
> `keyVaultSecretVersion` is the optional parameter. If not specified, it will be *current versoin*.
```
wget https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/keyvault/pv-blobfuse-csi-keyvault.yaml
vi pv-blobfuse-csi-keyvault.yaml
kubectl apply -f pv-blobfuse-csi-keyvault.yaml
```

## Create PVC 

```console
kubectl apply -f https://raw.githubusercontent.com/csi-driver/blobfuse-csi-driver/master/deploy/example/keyvault/pvc-blobfuse-csi-static-keyvault.yaml
```


