# Mount Azure Blob Storage with Managed Identity

This guide demonstrates how to use blobfuse mount with a user-assigned managed identity.

> **Tip:** You can use the built-in kubelet identity bound to the AKS agent node pool (named [`{AKS-Cluster-Name}-agentpool`](https://docs.microsoft.com/en-us/azure/aks/use-managed-identity#summary-of-managed-identities)). If you use your own managed identity, make sure it is assigned to the agent node pool.

## Prerequisites

### 0. Set environment variables

```bash
# Resource group where the managed identity resides
# (for kubelet identity, this is the node resource group, e.g., MC_myRG_myCluster_eastus)
export IDENTITY_RESOURCE_GROUP=<identity resource group>

# Name of the managed identity
export IDENTITY_NAME=<managed identity name>

# Resource group where the storage account resides (may differ from IDENTITY_RESOURCE_GROUP)
export STORAGE_RESOURCE_GROUP=<storage account resource group>

# Storage account name
export STORAGE_ACCOUNT_NAME=<storage account name>
```

### 1. Assign the `Storage Blob Data Contributor` role to the managed identity

The managed identity must have the **`Storage Blob Data Contributor`** role.

**For static provisioning** (bring your own storage account) — assign the role on the **storage account**:

```bash
# Get the principal ID of the managed identity
mid="$(az identity show -g "$IDENTITY_RESOURCE_GROUP" --name "$IDENTITY_NAME" --query principalId -o tsv)"

# Get the storage account resource ID
said="$(az storage account show -g "$STORAGE_RESOURCE_GROUP" --name "$STORAGE_ACCOUNT_NAME" --query id -o tsv)"

# Assign the role on the storage account
az role assignment create --assignee-object-id "$mid" --role "Storage Blob Data Contributor" --scope "$said"
```

**For dynamic provisioning** (driver creates the storage account) — assign the role on the **resource group** where the storage account will be created:

```bash
mid="$(az identity show -g "$IDENTITY_RESOURCE_GROUP" --name "$IDENTITY_NAME" --query principalId -o tsv)"
rgid="$(az group show -n "$STORAGE_RESOURCE_GROUP" --query id -o tsv)"

az role assignment create --assignee-object-id "$mid" --role "Storage Blob Data Contributor" --scope "$rgid"
```

### 2. Retrieve the client ID of the managed identity

> If you are using the kubelet identity, it is named `{aks-cluster-name}-agentpool` and located in the node resource group.

```bash
AzureStorageIdentityClientID=$(az identity show -g "$IDENTITY_RESOURCE_GROUP" --name "$IDENTITY_NAME" --query clientId -o tsv)
```

---

## Dynamic Provisioning

> The driver creates the container automatically. If `storageAccount` is not specified in the StorageClass, the driver also creates a new storage account.

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

1. Create a PV with the storage account name, container name, and managed identity client ID:

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
        # Note: the container name in volumeHandle must match containerName in volumeAttributes
        volumeHandle: "{resource-group-name}#{account-name}#{container-name}"
        volumeAttributes:
          protocol: fuse
          resourceGroup: EXISTING_RESOURCE_GROUP_NAME    # optional, node resource group if not provided
          storageAccount: EXISTING_STORAGE_ACCOUNT_NAME
          containerName: EXISTING_CONTAINER_NAME
          AzureStorageAuthType: MSI
          AzureStorageIdentityClientID: "xxxxx-xxxx-xxx-xxx-xxxxxxx"
    ```

2. Create a PVC and a Deployment with volume mount:

    ```bash
    kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/deployment.yaml
    ```
