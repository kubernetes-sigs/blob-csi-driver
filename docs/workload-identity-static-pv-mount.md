# Workload Identity Support for Static Provisioning

> Supported from v1.24.0 (AKS 1.29+ with `tokenRequests` field support in `CSIDriver`)

## Important Notes

- **NFS mount is not supported** — NFS does not require credentials during mount, so workload identity is not applicable.
- By default, this feature retrieves the storage account key using federated identity credentials.
- **Mount with workload identity token only (Preview):** Supported from v1.27.0. To enable:
  - Set `mountWithWorkloadIdentityToken: "true"` in `parameters` of the StorageClass or PersistentVolume
  - Grant **`Storage Blob Data Contributor`** role (instead of `Storage Account Contributor`) to the managed identity

---

## Prerequisites

### 1. Create an AKS cluster with OIDC issuer enabled and get credentials

Refer to the [documentation](https://learn.microsoft.com/en-us/azure/aks/use-oidc-issuer#create-an-aks-cluster-with-oidc-issuer) for creating a cluster with the `--enable-oidc-issuer` parameter.

```bash
export RESOURCE_GROUP=<your resource group name>
export CLUSTER_NAME=<your cluster name>

# Get cluster credentials for kubectl
az aks get-credentials --resource-group $RESOURCE_GROUP --name $CLUSTER_NAME
```

### 2. Bring your own storage account

Create a [new storage account and container](https://learn.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-cli), or use an existing one.

```bash
export STORAGE_RESOURCE_GROUP=<your storage account resource group>
export ACCOUNT=<your storage account name>
export CONTAINER=<your storage container name>  # required for static provisioning, optional for dynamic
```

### 3. Create a managed identity and assign a role

> **Tip:** You can use the built-in kubelet identity bound to the AKS agent node pool (named [`{AKS-Cluster-Name}-agentpool`](https://docs.microsoft.com/en-us/azure/aks/use-managed-identity#summary-of-managed-identities)) in the node resource group.

```bash
export UAMI=<your managed identity name>
az identity create --name $UAMI --resource-group $RESOURCE_GROUP

export USER_ASSIGNED_CLIENT_ID="$(az identity show -g $RESOURCE_GROUP --name $UAMI --query 'clientId' -o tsv)"
export ACCOUNT_SCOPE=$(az storage account show --name $ACCOUNT --query id -o tsv)
```

Choose **one** of the following role assignments:

| Mode | Role | Use Case |
|------|------|----------|
| **Account-key mode** (default) | `Storage Account Contributor` | Retrieves account key for mount |
| **Token-only mode** (Preview) | `Storage Blob Data Contributor` | Mounts using workload identity token only (v1.27.0+) |

**Account-key mode** (default):
```bash
az role assignment create --role "Storage Account Contributor" --assignee $USER_ASSIGNED_CLIENT_ID --scope $ACCOUNT_SCOPE
```

**Token-only mode** (Preview, v1.27.0+):
```bash
az role assignment create --role "Storage Blob Data Contributor" --assignee $USER_ASSIGNED_CLIENT_ID --scope $ACCOUNT_SCOPE
```

### 4. Create a Kubernetes service account

```bash
export SERVICE_ACCOUNT_NAME=<your sa name>
export SERVICE_ACCOUNT_NAMESPACE=<your sa namespace>

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${SERVICE_ACCOUNT_NAME}
  namespace: ${SERVICE_ACCOUNT_NAMESPACE}
EOF
```

### 5. Create the federated identity credential

Link the managed identity, OIDC issuer, and service account:

```bash
export FEDERATED_IDENTITY_NAME=<your federated identity name>
export AKS_OIDC_ISSUER="$(az aks show --resource-group $RESOURCE_GROUP --name $CLUSTER_NAME --query "oidcIssuerProfile.issuerUrl" -o tsv)"

az identity federated-credential create --name $FEDERATED_IDENTITY_NAME \
  --identity-name $UAMI \
  --resource-group $RESOURCE_GROUP \
  --issuer $AKS_OIDC_ISSUER \
  --subject system:serviceaccount:${SERVICE_ACCOUNT_NAMESPACE}:${SERVICE_ACCOUNT_NAME}
```

---

## Dynamic Provisioning with StorageClass

> The CSI driver control plane identity must have the **`Storage Account Contributor`** role on the storage account (or resource group if the account is created by the driver).
>
> AKS cluster control plane identity is assigned this role on the node resource group by default.

> **Note:** The example below uses `mountWithWorkloadIdentityToken: "true"` (token-only mode). If you chose this mode, make sure you assigned the `Storage Blob Data Contributor` role in step 3. If you prefer account-key mode (default), remove the `mountWithWorkloadIdentityToken` line.

```yaml
cat <<EOF | kubectl apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: blob-fuse-wi
provisioner: blob.csi.azure.com
parameters:
  storageaccount: $ACCOUNT                    # required
  clientID: $USER_ASSIGNED_CLIENT_ID          # required (for mount auth only)
  resourcegroup: $STORAGE_RESOURCE_GROUP      # optional, needed if account is outside MC_ resource group
  mountWithWorkloadIdentityToken: "true"      # token-only mode (Preview, v1.27.0+); remove for account-key mode
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
mountOptions:
  - -o allow_other
  - --file-cache-timeout-in-seconds=120
  - --use-attr-cache=true
  - --cancel-list-on-mount-seconds=10         # prevent billing charges on mounting
  - -o attr_timeout=120
  - -o entry_timeout=120
  - -o negative_timeout=120
  - --log-level=LOG_WARNING                   # LOG_WARNING, LOG_INFO, LOG_DEBUG
  - --cache-size-mb=1000                      # default is 80% of available memory
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: statefulset-blob
  namespace: ${SERVICE_ACCOUNT_NAMESPACE}
  labels:
    app: nginx
spec:
  serviceName: statefulset-blob
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      serviceAccountName: $SERVICE_ACCOUNT_NAME   # required for workload identity
      nodeSelector:
        kubernetes.io/os: linux
      containers:
        - name: statefulset-blob
          image: mcr.microsoft.com/mirror/docker/library/nginx:1.23
          command:
            - "/bin/bash"
            - "-c"
            - set -euo pipefail; while true; do echo $(date) >> /mnt/blob/outfile; sleep 1; done
          volumeMounts:
            - name: persistent-storage
              mountPath: /mnt/blob
  updateStrategy:
    type: RollingUpdate
  volumeClaimTemplates:
    - metadata:
        name: persistent-storage
      spec:
        storageClassName: blob-fuse-wi
        accessModes: ["ReadWriteMany"]
        resources:
          requests:
            storage: 100Gi
EOF
```

---

## Static Provisioning with PersistentVolume

```yaml
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolume
metadata:
  annotations:
    pv.kubernetes.io/provisioned-by: blob.csi.azure.com
  name: pv-blob
spec:
  capacity:
    storage: 10Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  storageClassName: blob-fuse
  mountOptions:
    - -o allow_other
    - --file-cache-timeout-in-seconds=120
  csi:
    driver: blob.csi.azure.com
    # make sure volumeHandle is unique for every storage blob container in the cluster
    volumeHandle: "{resource-group-name}#{account-name}#{container-name}"
    volumeAttributes:
      storageaccount: $ACCOUNT                   # required
      containerName: $CONTAINER                  # required
      clientID: $USER_ASSIGNED_CLIENT_ID         # required
      resourcegroup: $STORAGE_RESOURCE_GROUP     # optional, needed if account is outside MC_ resource group
      # mountWithWorkloadIdentityToken: "true"   # uncomment for token-only mode (Preview, v1.27.0+); requires Storage Blob Data Contributor role
---
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: pvc-blob
  namespace: ${SERVICE_ACCOUNT_NAMESPACE}
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 10Gi
  volumeName: pv-blob
  storageClassName: blob-fuse
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deployment-blob
  namespace: ${SERVICE_ACCOUNT_NAMESPACE}
  labels:
    app: nginx
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      serviceAccountName: $SERVICE_ACCOUNT_NAME   # required for workload identity
      nodeSelector:
        kubernetes.io/os: linux
      containers:
        - name: deployment-blob
          image: mcr.microsoft.com/oss/nginx/nginx:1.17.3-alpine
          command:
            - "/bin/sh"
            - "-c"
            - while true; do echo $(date) >> /mnt/blob/outfile; sleep 1; done
          volumeMounts:
            - name: blob
              mountPath: /mnt/blob
      volumes:
        - name: blob
          persistentVolumeClaim:
            claimName: pvc-blob
  strategy:
    rollingUpdate:
      maxSurge: 0
      maxUnavailable: 1
    type: RollingUpdate
EOF
```
