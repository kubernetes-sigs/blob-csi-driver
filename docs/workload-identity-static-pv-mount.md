# workload identity support on static provisioning
 - supported from v1.24.0 (from AKS 1.29 with `tokenRequests` field support in `CSIDriver`)

### Note
 - This feature is not supported for NFS mount since NFS mount does not need credentials.
 - This feature would retrieve storage account key using federated identity credentials by default.
 - This feature supports mounting with workload identity token only (**Preview**) by configuring the following:
    > limitation: the workload identity token would expire after 24 hours, make sure the blobfuse volume would be remounted by your application before it expires
    - set `mountWithWorkloadIdentityToken: "true"` in parameters of storage class or persistent volume
    - set `Storage Blob Data Contributor` role on the identity

## Prerequisites
### 1. Create a cluster with oidc-issuer enabled and get the credential

Following the [documentation](https://learn.microsoft.com/en-us/azure/aks/use-oidc-issuer#create-an-aks-cluster-with-oidc-issuer) to create an AKS cluster with the `--enable-oidc-issuer` parameter and get the AKS credentials. And export following environment variables:
```
export RESOURCE_GROUP=<your resource group name>
export CLUSTER_NAME=<your cluster name>
export REGION=<your region>
```

### 2. Bring your own storage account and storage container
Following the [documentation](https://learn.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-cli) to create a new storage account and container or use your own. And export following environment variables:
```
export STORAGE_RESOURCE_GROUP=<your storage account resource group>
export ACCOUNT=<your storage account name>
export CONTAINER=<your storage container name>
```

### 3. Create managed identity and role assignment
```
export UAMI=<your managed identity name>
az identity create --name $UAMI --resource-group $RESOURCE_GROUP

export USER_ASSIGNED_CLIENT_ID="$(az identity show -g $RESOURCE_GROUP --name $UAMI --query 'clientId' -o tsv)"
export IDENTITY_TENANT=$(az aks show --name $CLUSTER_NAME --resource-group $RESOURCE_GROUP --query identity.tenantId -o tsv)
export ACCOUNT_SCOPE=$(az storage account show --name $ACCOUNT --query id -o tsv)

# please retry if you meet `Cannot find user or service principal in graph database` error, it may take a while for the identity to propagate
az role assignment create --role "Storage Account Contributor" --assignee $USER_ASSIGNED_CLIENT_ID --scope $ACCOUNT_SCOPE
```

### 4. Create service account on AKS
```
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

### 5. Create the federated identity credential between the managed identity, service account issuer, and subject using the `az identity federated-credential create` command.
```
export FEDERATED_IDENTITY_NAME=<your federated identity name>
export AKS_OIDC_ISSUER="$(az aks show --resource-group $RESOURCE_GROUP --name $CLUSTER_NAME --query "oidcIssuerProfile.issuerUrl" -o tsv)"

az identity federated-credential create --name $FEDERATED_IDENTITY_NAME \
--identity-name $UAMI \
--resource-group $RESOURCE_GROUP \
--issuer $AKS_OIDC_ISSUER \
--subject system:serviceaccount:${SERVICE_ACCOUNT_NAMESPACE}:${SERVICE_ACCOUNT_NAME}
```
## option#1: dynamic provisioning with storage class
```
cat <<EOF | kubectl apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: blob-fuse
provisioner: blob.csi.azure.com
parameters:
  storageaccount: $ACCOUNT # required
  clientID: $USER_ASSIGNED_CLIENT_ID # required, $USER_ASSIGNED_CLIENT_ID is only for mount auth, make sure you CSI driver controller pod has `Contributor` role on the specified account
  resourcegroup: $STORAGE_RESOURCE_GROUP # optional, specified when the storage account is not under AKS node resource group(which is prefixed with "MC_")
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
  template:
    metadata:
      labels:
        app: nginx
    spec:
      serviceAccountName: $SERVICE_ACCOUNT_NAME  #required, Pod lacks the necessary permission to mount the volume without this field
      nodeSelector:
        "kubernetes.io/os": linux
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
              readOnly: false
  updateStrategy:
    type: RollingUpdate
  selector:
    matchLabels:
      app: nginx
  volumeClaimTemplates:
    - metadata:
        name: persistent-storage
      spec:
        storageClassName: blob-fuse
        accessModes: ["ReadWriteMany"]
        resources:
          requests:
            storage: 100Gi
EOF
```

## option#2: static provision with PV
```
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
      storageaccount: $ACCOUNT # required
      containerName: $CONTAINER  # required
      clientID: $USER_ASSIGNED_CLIENT_ID # required
      resourcegroup: $STORAGE_RESOURCE_GROUP # optional, specified when the storage account is not under AKS node resource group(which is prefixed with "MC_")
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
  labels:
    app: nginx
  name: deployment-blob
  namespace: ${SERVICE_ACCOUNT_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
      name: deployment-blob
    spec:
      serviceAccountName: $SERVICE_ACCOUNT_NAME  #required, Pod lacks the necessary permission to mount the volume without this field
      nodeSelector:
        "kubernetes.io/os": linux
      containers:
        - name: deployment-blob
          image: mcr.microsoft.com/oss/nginx/nginx:1.17.3-alpine
          command:
            - "/bin/sh"
            - "-c"
            - while true; do echo $(date) >> /mnt/blob/outfile; sleep 1; done
          volumeMounts:
            - name: blob
              mountPath: "/mnt/blob"
              readOnly: false
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
