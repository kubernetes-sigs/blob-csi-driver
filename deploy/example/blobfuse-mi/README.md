# Mount an azure blob storage

In case you have the requirement, that your AKS cluster has to access a blob storage with kubelet identity or a dedicated user-assigned managed identity, the following solution will do this.

You can also use a different managed-identity for different persistent volumes (f.e. you have a pod, that should just have write access to some objects while having another pod, that should have write access everywhere.)


## Before you begin

- The Azure CLI version 2.37.0 or later. Run `az --version` to find the version, and run `az upgrade` to upgrade the version. If you need to install or upgrade, see [Install Azure CLI][install-azure-cli].

- Install the aks-preview Azure CLI extension version 0.5.85 or later.

- Ensure, that you are authenticated or run `az login` 

- Run `az account set --subscription "mysubscription"` to select the right subscription

- Create a storage account container(optional in dynamic provisioning), e.g.
    ```bash
    resourcegroup="blobfuse-mi"
    storageaccountname="myaksblob"
    az storage account create -g "$resourcegroup" -n "$storageaccountname" --access-tier Hot  --sku Standard_LRS
    az storage container create -n mycontainer --account-name "$storageaccountname" --public-access off
    ```

- Get the clientID for `AzureStorageIdentityClientID`. If you use kubelet identity, the identity name is `blobfuse-mi-agentpool`, and the resourcegroup is node resourcegroup, or you can use a dedicated user-assigned managed identity
    ```bash
    az identity list -g "$resourcegroup" --query "[?name == '$identityname'].clientId" -o tsv
    ```
    
## dynamic provisioning in an existing resource group

1. Grant cluster system assigned identity(control plane identity) `Storage Account Contributor` role to resource group, if mount in an existing storage account, then should also grant identities to storage account

1. Grant kubelet identity `Storage Blob Data Owner` role to resource group to mount blob storage, if mount in an existing storage account, then should also grant identity to storage account

1. Create a storage class in an existing resource group
    - Option#1 create storage account by CSI driver, will create a new storage account when `storageAccount` and `containerName` are not provided.
    - Option#2 use your own storage account, set storage account name for `storageAccount`, you can also set an existing container name for `containerName` if you want to mount an existing container.
    ```yml
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: blob-fuse
    provisioner: blob.csi.azure.com
    parameters:
      skuName: Premium_LRS 
      protocol: fuse
      resourceGroup: EXISTING_RESOURCE_GROUP_NAME
      storageAccount: EXISTING_STORAGE_ACCOUNT_NAME # optional, if use existing storage account
      containerName: EXISTING_CONTAINER_NAME # optional, if use existing container
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

1. Create application
    - Create a statefulset with volume mount
      ```console
      kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/statefulset.yaml
      ```

    - Execute `df -h` command in the container
      ```console
      kubectl exec -it statefulset-blob-0 -- df -h
      ```
      <pre>
      Filesystem      Size  Used Avail Use% Mounted on
      ...
      blobfuse         14G   41M   13G   1% /mnt/blob
      ...
      </pre>

## static provisioning(use an existing storage account)
### Option#1: grant kubelet identity access to storage account

1. Give kubelet identity access to storage account
    ```bash
    aksnprg="$(az aks list -g "$resourcegroup" --query "[?name == '$aksname'].nodeResourceGroup" -o tsv)"
    kloid="$(az identity list -g "$aksnprg" --query "[?name == 'blobfuse-mi-agentpool'].principalId" -o tsv)"
    said="$(az storage account list -g "$resourcegroup" --query "[?name == '$storageaccountname'].id" -o tsv)"
    az role assignment create --assignee-object-id "$kloid" --role "Storage Blob Data Owner" --scope "$said"
    ```

1. Get the clientID of kubelet identity
    ```bash
    az identity list -g "$aksnprg" --query "[?name == 'blobfuse-mi-agentpool'].clientId" -o tsv
    ```

### Option#2: grant a dedicated user-assigned managed identity access to storage account
You can use a dedicated user-assigned managed identity to mount the storage.

1. Create user-assigned managed identity and give access to storage account
    ```bash
    az identity create -n myaksblobmi -g "$resourcegroup"
    miioid="$(az identity list -g "$resourcegroup" --query "[?name == 'myaksblobmi'].principalId" -o tsv)"
    said="$(az storage account list -g "$resourcegroup" --query "[?name == '$storageaccountname'].id" -o tsv)"
    az role assignment create --assignee-object-id "$miioid" --role "Storage Blob Data Owner" --scope "$said"
    ```

1. Assign the user-assigned managed identity to the AKS vm scale set (system nodepool)
    ```bash
    aksnprg="$(az aks list -g "$resourcegroup" --query "[?name == '$aksname'].nodeResourceGroup" -o tsv)"
    aksnp="$(az vmss list -g "$aksnprg" --query "[?starts_with(name, 'aks-nodepool1-')].name" -o tsv)"
    miid="$(az identity list -g "$resourcegroup" --query "[?name == 'myaksblobmi'].id" -o tsv)"
    az vmss identity assign -g "$aksnprg" -n "$aksnp" --identities "$miid"
    ```

1. Get the clientID of your user-assigned managed identity
    ```bash
    az identity list -g "$resourcegroup" --query "[?name == 'myaksblobmi'].clientId" -o tsv
    ```

### Mount the azure blob storage

1. Create storage class
    ```console
    kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blobfuse.yaml
    ```

1. Create PV and set clientID for ``AzureStorageIdentityClientID``. Please also check ``resourceGroup`` and ``storageAccount``.
    ```yml
    apiVersion: v1
    kind: PersistentVolume
    metadata:
      name: pv-blob
    spec:
      capacity:
        storage: 10Gi
      accessModes:
        - ReadWriteMany
      persistentVolumeReclaimPolicy: Retain  # If set as "Delete" container would be removed after pvc deletion
      storageClassName: blob-fuse
      mountOptions:
        - -o allow_other
        - --file-cache-timeout-in-seconds=120
      csi:
        driver: blob.csi.azure.com
        readOnly: false
        # make sure this volumeid is unique in the cluster
        # `#` is not allowed in self defined volumeHandle
        volumeHandle: pv-blob
        volumeAttributes:
          protocol: fuse
          resourceGroup: blobfuse-mi
          storageAccount: myaksblob
          containerName: mycontainer
          AzureStorageAuthType: MSI
          AzureStorageIdentityClientID: "xxxxx-xxxx-xxx-xxx-xxxxxxx"
    ```

1. Create PVC and a deployment with volume mount
    ```console
    kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/deployment.yaml
    # check pod
    kubectl get pods
    ```

## how to add another pv with a dedicated user-assigned identity?

1. Create another user-assigned managed identity and give access to storage account
    ```bash
    az identity create -n myaksblobmi2 -g "$resourcegroup"
    miioid="$(az identity list -g "$resourcegroup" --query "[?name == 'myaksblobmi2'].principalId" -o tsv)"
    said="$(az storage account list -g "$resourcegroup" --query "[?name == '$storageaccountname'].id" -o tsv)"
    az role assignment create --assignee-object-id "$miioid" --role "Storage Blob Data Reader" --scope "$said"
    ```

1. Assign the user-assigned managed identity to the AKS vm scale set (system nodepool)
    ```bash
    aksnprg="$(az aks list -g "$resourcegroup" --query "[?name == '$aksname'].nodeResourceGroup" -o tsv)"
    aksnp="$(az vmss list -g "$aksnprg" --query "[?starts_with(name, 'aks-nodepool1-')].name" -o tsv)"
    miid="$(az identity list -g "$resourcegroup" --query "[?name == 'myaksblobmi2'].id" -o tsv)"
    az vmss identity assign -g "$aksnprg" -n "$aksnp" --identities "$miid"
    ```

1. Get the objectID of your user-assigned managed identity
    ```bash
    az identity list -g -g "$resourcegroup" --query "[?name == 'myaksblobmi2'].principalId" -o tsv
    ```

1. Create storage class
    ```console
    kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/storageclass-blobfuse.yaml
    ```

1. Create PV and set objectID for ``AzureStorageIdentityClientID``. \
   Please also check ``resourceGroup`` and ``storageAccount``.
    ```yml
    apiVersion: v1
    kind: PersistentVolume
    metadata:
      name: pv-blob
    spec:
      capacity:
        storage: 10Gi
      accessModes:
        - ReadWriteMany
      persistentVolumeReclaimPolicy: Retain  # If set as "Delete" container would be removed after pvc deletion
      storageClassName: blob-fuse
      mountOptions:
        - -o allow_other
        - --file-cache-timeout-in-seconds=120
      csi:
        driver: blob.csi.azure.com
        readOnly: false
        # make sure this volumeid is unique in the cluster
        # `#` is not allowed in self defined volumeHandle
        volumeHandle: pv-blob
        volumeAttributes:
          protocol: fuse
          resourceGroup: blobfuse-mi
          storageAccount: myaksblob
          containerName: mycontainer
          AzureStorageAuthType: MSI
          AzureStorageIdentityClientID: "xxxxx-xxxx-xxx-xxx-xxxxxxx"
    ```

1. Create PVC
    ```console
    kubectl create -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/pvc-blob-csi-static.yaml
    # make sure pvc is created and in Bound status after a while
    kubectl describe pvc pvc-blob
    ```

1. Now you can use the persistent volume claim ``pv-blob`` in another deployment.
