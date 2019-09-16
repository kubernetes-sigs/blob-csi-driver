# Use Blobfuse CSI Driver with Azure Key Vault

> Attention: Currently, we just support use Key Vault in static provisioning scenario.

## Prepare Key Vault

1. Create a Key Vault in the [portal](https://ms.portal.azure.com/#blade/HubsExtension/BrowseResourceBlade/resourceType/Microsoft.KeyVault%2Fvaults).

2. Store `storage account key` or `SAS token` in Key Vault's Secret.

3. Ensure the service principal has all the required permissions to access content in your Azure key vault instance. If not, you can run the following using the Azure CLI:

   ```console
   # Assign Reader Role to the service principal for your keyvault
   az role assignment create --role Reader --assignee <aadClientId> --scope /subscriptions/<subscriptionid>/resourcegroups/<resourcegroup>/providers/Microsoft.KeyVault/vaults/<keyvaultname>
   
   az keyvault set-policy -n $KV_NAME --key-permissions get --spn <YOUR SPN CLIENT ID>
   az keyvault set-policy -n $KV_NAME --secret-permissions get --spn <YOUR SPN CLIENT ID>
   az keyvault set-policy -n $KV_NAME --certificate-permissions get --spn <YOUR CLIENT ID>
   ```

## Install Blobfuse CSI Driver

### Option #1

Use the [script](https://github.com/csi-driver/blobfuse-csi-driver/blob/master/deploy/install-driver.sh) to install.

### Option #2

Use [helm](https://github.com/csi-driver/blobfuse-csi-driver/blob/master/charts/README.md) to install.

## Create PVC 

Use default pvc file to create.

```console
kubectl apply -f pvc-blobfuse-csi-static-keyvault.yaml
```

## Create PV

1. Replace your Key Vault infomation in the yaml.

   `keyVaultURL`  and `keyVaultSecretName` are the required parameters.

   `keyVaultSecretVersion` is the optional parameter. If not specified, it will be *current versoin*.
2. Create pv 

    ```console
    kubectl apply -f pv-blobfuse-csi-static-keyvault.yaml
    ```
