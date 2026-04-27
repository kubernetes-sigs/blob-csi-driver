# Mount Azure Blob Storage with Managed Identity

This guide demonstrates how to use blobfuse mount with a user-assigned managed identity.

> **Tip:** You can use the built-in kubelet identity bound to the AKS agent node pool (named [`{AKS-Cluster-Name}-agentpool`](https://docs.microsoft.com/en-us/azure/aks/use-managed-identity#summary-of-managed-identities)). If you use your own managed identity, make sure it is assigned to the agent node pool.

## Prerequisites

### 1. Assign the `Storage Blob Data Contributor` role to the managed identity

The managed identity must have the **`Storage Blob Data Contributor`** role on the storage account (for static provisioning / bring-your-own storage account) or on the resource group (for dynamic provisioning where the driver creates the storage account).

```bash
# Get the principal ID of the managed identity
mid="$(az identity list -g "$resourcegroup" --query "[?name == 'managedIdentityName'].principalId" -o tsv)"

# Get the storage account resource ID
said="$(az storage account list -g "$resourcegroup" --query "[?name == '$storageaccountname'].id" -o tsv)"

# Assign the role
az role assignment create --assignee-object-id "$mid" --role "Storage Blob Data Contributor" --scope "$said"
```

### 2. Retrieve the client ID of the managed identity

> If you are using the kubelet identity, it is named `{aks-cluster-name}-agentpool` and located in the node resource group.

```bash
AzureStorageIdentityClientID=$(az identity list -g "$resourcegroup" --query "[?name == '$identityname'].clientId" -o tsv)
```

---

## Dynamic Provisioning

> The driver creates the storage account and container automatically.

### Additional role requirement

The CSI driver control plane identity must have the **`Storage Account Contributor`** role on the resource group where the storage account is located.

> AKS cluster control plane identity is assigned the `Storage Account Contributor` role on the node resource group by default.

### Steps

1. Create a StorageClass:

    ```yaml
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: blob-fuse
    provisioner: blob.csi.azure.com
    parameters:
      skuName: Premium_LRS
      protocol: fuse
      resourceGroup: EXISTING_RESOURCE_GROUP_NAME    # optional, node resource group by default
      storageAccount: EXISTING_STORAGE_ACCOUNT_NAME  # optional, a new account will be created if not provided
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

2. Create a StatefulSet with volume mount:

    ```bash
    kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/statefulset.yaml
    ```

---

## Static Provisioning

> Bring your own storage account and container.

### Steps

1. Create a PV with the storage account name and managed identity client ID:

    ```yaml
    apiVersion: v1
    kind: PersistentVolume
    metadata:
      name: pv-blob
    spec:
      capacity:
        storage: 100Gi
      accessModes:
        - ReadWriteMany
      persistentVolumeReclaimPolicy: Retain  # "Delete" removes the container after PVC deletion
      storageClassName: blob-fuse
      mountOptions:
        - -o allow_other
        - --file-cache-timeout-in-seconds=120
      csi:
        driver: blob.csi.azure.com
        # make sure this volumeid is unique in the cluster
        volumeHandle: "{resource-group-name}#{account-name}#{container-name}"
        volumeAttributes:
          protocol: fuse
          resourceGroup: EXISTING_RESOURCE_GROUP_NAME    # optional, node resource group if not provided
          storageAccount: EXISTING_STORAGE_ACCOUNT_NAME
          AzureStorageAuthType: MSI
          AzureStorageIdentityClientID: "xxxxx-xxxx-xxx-xxx-xxxxxxx"
    ```

2. Create a PVC and a Deployment with volume mount:

    ```bash
    kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/deployment.yaml
    ```
