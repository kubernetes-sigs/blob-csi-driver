# Getting started with BlobFuse2 and DACS on AKS (Preview)

Mount an existing Azure Blob container into an AKS workload with a shared,
distributed read cache. This walkthrough downloads
[Microsoft Phi-1.5](https://huggingface.co/microsoft/phi-1_5), uploads it to Blob
Storage, and verifies that a reader on a second node can reuse cached data.

[Distributed Accelerator for Cloud Storage (DACS)](https://github.com/Azure/dacs)
is an open-source distributed caching system. BlobFuse2's `distributed_cache`
component lets applications use it through a mounted volume without a
DACS-specific API. Cache misses fetch data from Blob Storage and populate DACS
asynchronously so other workloads can reuse it.

```text
AKS workload
    |
    | reads files from a mounted volume
    v
Blob CSI driver -> BlobFuse2
                      |
                      +-- shared DACS cache
                      |
                      +-- Azure Blob Storage on a cache miss
```

## 1. Check eligibility before creating resources

### Compatibility and deployment choice

| Component | Requirement for this preview |
| --- | --- |
| AKS version | 1.34 or later for the managed Blob CSI add-on path |
| Reader node OS | Ubuntu 20.04, 22.04, or 24.04 |
| Blob CSI driver | Managed Blob CSI add-on enabled with distributed-cache support, introduced in 1.27.11 |
| Node-host BlobFuse2 | 2.5.6, which introduces distributed cache in preview |
| Volume access | Read-only mounts only |

The main path uses the **AKS-managed Blob CSI add-on** on an existing cluster.
The add-on uses BlobFuse2 installed in the node image rather than installing
or pinning the binary itself.

For a cluster without the managed add-on, the
[open-source driver alternative](#open-source-blob-csi-driver) is also available.

> [!WARNING]
> DACS assumes the entire Kubernetes cluster is a single trust domain and does
> not provide tenant isolation. Do not use this evaluation in a multi-tenant
> cluster. Review the [DACS security model](https://github.com/Azure/dacs/blob/main/docs/security-model.md).

### Cluster, tools, and permissions

Before downloading the model, confirm all of the following:

- You have at least **two supported Ubuntu worker nodes** for readers, plus
  capacity for a DACS controller and **two 2 GiB ephemeral CacheServers**
  (4 GiB total configured cache capacity). CacheServers may share reader nodes
  if CPU, memory, and backing storage permit.
  This is an evaluation topology, not a production sizing recommendation.
  For resource requirements, NVMe/RAID preparation, and operations, use the
  [DACS deployment guide](https://github.com/Azure/dacs/blob/main/user-guide.md).
  DACS is pre-release software.
- Your workstation has Bash, Helm 3, `kubectl`, Azure CLI (`az`),
  [Hugging Face CLI](https://huggingface.co/docs/huggingface_hub/guides/cli)
  (`hf`), `sha256sum`, `awk`, `sort`, `tee`, and `mktemp`.
  No repository checkout is required; run the inline commands from any directory.
- Your workstation can reach Hugging Face and the storage account and has at
  least 4 GB of free space for the model. Cluster nodes can pull the driver,
  DACS, and BusyBox images, reach Blob Storage, and communicate with DACS.
  The DACS charts and images at `dacstest.azurecr.io` are publicly readable.
- Your Kubernetes identity can install cluster-scoped resources, including
  DACS CRDs, and create namespaces, Secrets, PVs, PVCs, and pods.
  It can also list CacheServer pods and execute commands in their
  `cacheserver` containers. The inline metrics check requires `curl` in those
  containers; it does not require Prometheus.
- You have an existing storage account and Blob container, an upload identity
  with **Storage Blob Data Contributor** or equivalent access, and network
  access from the reader nodes.
- For the main path, the storage account permits **shared-key authentication**
  and you can obtain an account key. The key authenticates the mount separately
  from your upload identity. The read-only mount does not reduce the account
  key's permissions.

If account keys are prohibited, adapt the
[identity-based alternative](#identity-based-storage-access) before proceeding.

## 2. Set shared configuration and check the cluster

Use **terminal B** for setup and model reads. Replace the five placeholders
below. `COLD_NODE` and `WARM_NODE` must be different `kubernetes.io/hostname`
label values in the selected Ubuntu node pool. First inspect your context and
nodes:

```bash
kubectl config current-context
kubectl cluster-info
kubectl get nodes -o wide -L kubernetes.azure.com/agentpool,kubernetes.io/hostname
```

Set the configuration once. The commands save the named, non-secret settings
in a local working directory for reuse across terminals.

```bash
set -euo pipefail

export STORAGE_ACCOUNT='<storage-account-name>'
export STORAGE_CONTAINER='<container-name>'
export NODE_POOL='<supported-ubuntu-node-pool>'
export COLD_NODE='<first-node-hostname-label>'
export WARM_NODE='<second-node-hostname-label>'
export DACS_NAMESPACE='dacs-system'
export DACS_CACHE='cache-sample'
export DACS_ENDPOINT="$DACS_CACHE-discovery.$DACS_NAMESPACE.svc.cluster.local:9065"
export DEMO_DIR
DEMO_DIR=$(mktemp -d "$HOME/blob-dacs-demo.XXXXXX")
export MODEL_REVISION='77aa61eeac94fbf33d492b9f2744c98b42d5b5eb'
export MODEL_SHA256='b4f562cc4b12f1154a0b658b9d74ab9d024a686420fdf154ebc00a2219976df7'
export MODEL_DIR="$DEMO_DIR/phi-1_5-$MODEL_REVISION"
export BLOB_PREFIX="models/phi-1_5/$MODEL_REVISION/gsg-$(date -u +%Y%m%dT%H%M%SZ)-$RANDOM"
export MODEL_PATH="/mnt/blob/$BLOB_PREFIX/model.safetensors"
export CLUSTER_DNS
CLUSTER_DNS=$(kubectl get service kube-dns -n kube-system \
  -o jsonpath='{.spec.clusterIP}')

test -n "$CLUSTER_DNS"
test "$COLD_NODE" != "$WARM_NODE"

declare -p STORAGE_ACCOUNT STORAGE_CONTAINER NODE_POOL \
  COLD_NODE WARM_NODE DACS_NAMESPACE DACS_CACHE DACS_ENDPOINT DEMO_DIR \
  MODEL_REVISION MODEL_SHA256 MODEL_DIR BLOB_PREFIX MODEL_PATH CLUSTER_DNS \
  > "$DEMO_DIR/settings.env"
printf 'Restore settings in either terminal: source %q\n' "$DEMO_DIR/settings.env"

kubectl get nodes -l "kubernetes.azure.com/agentpool=$NODE_POOL" \
  -o wide -L kubernetes.io/hostname
kubectl auth can-i create persistentvolumes
kubectl auth can-i create customresourcedefinitions.apiextensions.k8s.io
kubectl auth can-i create pods/exec -n "$DACS_NAMESPACE"

az login
az account show --query '{subscription:name,subscriptionId:id}' -o table
az storage container show --account-name "$STORAGE_ACCOUNT" \
  --name "$STORAGE_CONTAINER" --auth-mode login -o table
```

**Checkpoint:** both selected nodes are Ready and in the supported pool, the
permission checks return `yes`, and the account/container lookup succeeds.
Resolve any failures before continuing; these checks cover only part of the
prerequisites. On reconnect, source the printed settings file rather than
rerunning this block, which generates a new blob prefix.

The discovery endpoint assumes the standard `cluster.local` DNS domain.
Adjust `DACS_ENDPOINT` before saving settings if your cluster uses another
domain. The PV below explicitly uses your cluster DNS IP because the driver's
default `10.0.0.10` is fixed, not automatically discovered.

## 3. Install and verify the driver and DACS

### Verify the AKS-managed Blob CSI add-on

Confirm the add-on is enabled in your cluster's AKS configuration; see the
[AKS driver documentation](https://learn.microsoft.com/azure/aks/azure-blob-csi)
if it needs to be enabled. If you chose the
[open-source alternative](#open-source-blob-csi-driver), install it first, then
return to these verification commands:

```bash
kubectl rollout status daemonset/csi-blob-node -n kube-system --timeout=10m
kubectl get daemonset csi-blob-node -n kube-system \
  -o jsonpath='{range .spec.template.spec.containers[*]}{.name}{": "}{.image}{"\n"}{end}'
```

**Checkpoint:** the node DaemonSet is ready on both reader nodes and its `blob`
image meets the compatibility requirements in step 1. On **each reader
node's host**, run the following using your approved node-access method in the
[CSI debugging guide](./csi-debug.md):

```bash
blobfuse2 -v
```

Expect `blobfuse2 version 2.5.6`. Checking a workstation or container binary
does not verify the node-host binary used by the proxy. Stop if versions do
not meet the requirements. If the node image lacks the required binary, use
a supported image or wait for availability.

### Install two ephemeral DACS CacheServers

Use a **dedicated evaluation cache** with no other readers during measurement.
Do not resize a shared cache for this test.
The model weights are about 2.64 GiB; the upstream quickstart's default 2 GiB
single CacheServer is too small. Use **two CacheServers with the default 2 GiB
each**, for 4 GiB total configured capacity. Keep this cache dedicated to the
evaluation; the model and the quickstart's 1 GiB smoke-test data leave limited
headroom, and data placement and eviction can affect warm-read results.

Follow these sections of the [DACS AKS quickstart](https://github.com/Azure/dacs/blob/main/docs/quickstart.md),
in order:

1. **Set variables** and **Resolve matching chart and image versions**. Use
   `dacs-system` as the namespace and retain the resolved chart/image versions.
2. **Install DACS and an ephemeral cache**, with the change below.
3. **Verify the deployment**.
4. **Run the end-to-end smoke test** and **Confirm success**.

In the standalone `Cache` manifest, set **`spec.scaling.static.numServers` to
`2`** and retain the default **`spec.serverSizeInGB: 2`** before applying it:

```yaml
spec:
  scaling:
    static:
      numServers: 2
  serverSizeInGB: 2
```

This is a manifest fragment, not a complete resource. Do not substitute
Helm `--set cache.*` values for these manifest changes: the quickstart creates
its Cache separately from the controller chart.

**Do not run the upstream cleanup yet.** Return here after both smoke-test
Jobs complete successfully and report `All iterations succeeded: true`.

```bash
kubectl get cache "$DACS_CACHE" -n "$DACS_NAMESPACE" \
  -o jsonpath='{"servers="}{.spec.scaling.static.numServers}{" size-per-server-GiB="}{.spec.serverSizeInGB}{"\n"}'
kubectl wait statefulset/"$DACS_CACHE" -n "$DACS_NAMESPACE" \
  --for=jsonpath='{.status.readyReplicas}'=2 --timeout=10m
kubectl get pods -n "$DACS_NAMESPACE" \
  -l "dacs.azure.com/cache-name=$DACS_CACHE" -o wide
kubectl get service "$DACS_CACHE-discovery" -n "$DACS_NAMESPACE"
```

**Checkpoint:** the configuration reports `servers=2 size-per-server-GiB=2`,
both CacheServers are Ready, the discovery service exists, and the smoke test
passed. The inline metrics check in step 6 queries both CacheServers.

## 4. Configure storage access and upload the sample

### Create the mount credentials first

Use a new namespace dedicated to this walkthrough. If `blob-dacs-demo` already
exists, stop and check its ownership rather than reusing unknown resources.
Enter your storage account key at the prompt; do not paste it into this
document or the saved settings file.

```bash
read -r -s -p 'Storage account key: ' STORAGE_KEY
printf '\n'
test -n "$STORAGE_KEY"
AZURE_STORAGE_KEY="$STORAGE_KEY" az storage container show \
  --account-name "$STORAGE_ACCOUNT" \
  --name "$STORAGE_CONTAINER" --auth-mode key -o table
kubectl create namespace blob-dacs-demo
kubectl create secret generic azure-secret \
  --namespace blob-dacs-demo \
  --from-literal=azurestorageaccountname="$STORAGE_ACCOUNT" \
  --from-literal=azurestorageaccountkey="$STORAGE_KEY"
unset STORAGE_KEY
```

**Checkpoint:** the account-key container lookup succeeds before any model
transfer. If it fails, check the key, shared-key policy, and storage network
access; do not continue to the download.

Do not enable shell tracing while handling credentials. The key is passed to
the local `kubectl` process; follow your organization's credential-handling
requirements or use an identity-based alternative.

### Download and upload Phi-1.5

Review the model's [model card and MIT license](https://huggingface.co/microsoft/phi-1_5)
before use. The revision and checksum pin identical bytes for both reads.

```bash
hf download microsoft/phi-1_5 \
  --revision "$MODEL_REVISION" --local-dir "$MODEL_DIR"
printf '%s  %s\n' "$MODEL_SHA256" "$MODEL_DIR/model.safetensors" \
  | sha256sum --check

az storage blob upload-batch \
  --account-name "$STORAGE_ACCOUNT" \
  --auth-mode login \
  --destination "$STORAGE_CONTAINER" \
  --destination-path "$BLOB_PREFIX" \
  --source "$MODEL_DIR" \
  --pattern '[!.]*' \
  --overwrite false

printf 'Model path for both readers: %s\n' "$MODEL_PATH"
```

**Checkpoint:** the local checksum reports `OK` and the upload succeeds.
The upload pattern excludes hidden download metadata, such as `.cache`.

## 5. Create the read-only volume and both readers

### Create the PersistentVolume and claim

Run this Bash block in terminal B. It expands the saved configuration into
the manifest; there are no additional placeholders to replace.

```bash
kubectl apply -f - <<YAML
apiVersion: v1
kind: PersistentVolume
metadata:
  name: pv-blob-dacs-demo
spec:
  capacity:
    storage: 10Gi
  accessModes:
    - ReadOnlyMany
  persistentVolumeReclaimPolicy: Retain
  storageClassName: ""
  mountOptions:
    - -o allow_other
    - -o ro
    - --read-only=true
    - --distributed-cache-discovery-endpoint=$DACS_ENDPOINT
    - --distributed-cache-dns-server=$CLUSTER_DNS
  csi:
    driver: blob.csi.azure.com
    readOnly: true
    volumeHandle: "$STORAGE_ACCOUNT#$STORAGE_CONTAINER#blob-dacs-demo"
    volumeAttributes:
      protocol: fuse2
      storageAccount: "$STORAGE_ACCOUNT"
      containerName: "$STORAGE_CONTAINER"
    nodeStageSecretRef:
      name: azure-secret
      namespace: blob-dacs-demo
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: pvc-blob-dacs-demo
  namespace: blob-dacs-demo
spec:
  accessModes:
    - ReadOnlyMany
  storageClassName: ""
  volumeName: pv-blob-dacs-demo
  resources:
    requests:
      storage: 10Gi
YAML

kubectl wait pvc/pvc-blob-dacs-demo -n blob-dacs-demo \
  --for=jsonpath='{.status.phase}'=Bound --timeout=2m
```

The empty storage class and explicit `volumeName` select static binding.
The PV's capacity is for Kubernetes binding; it does not size DACS or impose
a Blob container quota. The discovery endpoint enables the distributed-cache
pipeline. Do not add file-cache or streaming options.

### Create readers on different nodes

The helper below creates both pods from the same manifest.

```bash
create_reader() {
  local name="$1" node="$2"
  kubectl apply -f - <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: $name
  namespace: blob-dacs-demo
spec:
  nodeSelector:
    kubernetes.io/os: linux
    kubernetes.azure.com/agentpool: "$NODE_POOL"
    kubernetes.io/hostname: "$node"
  containers:
    - name: reader
      image: busybox:1.36
      command: ["sh", "-c", "sleep 86400"]
      volumeMounts:
        - name: dataset
          mountPath: /mnt/blob
          readOnly: true
  volumes:
    - name: dataset
      persistentVolumeClaim:
        claimName: pvc-blob-dacs-demo
        readOnly: true
YAML
}

create_reader blob-dacs-reader "$COLD_NODE"
create_reader blob-dacs-reader-warm "$WARM_NODE"
kubectl wait pod/blob-dacs-reader pod/blob-dacs-reader-warm \
  -n blob-dacs-demo --for=condition=Ready --timeout=5m
kubectl get pods -n blob-dacs-demo -o wide
kubectl exec -n blob-dacs-demo blob-dacs-reader -- ls /mnt/blob
```

**Checkpoint:** the PVC is Bound and both pods are Ready on **different nodes**.
Listing directories is fine; **do not read the weights yet**.

## 6. Verify cold and warm reads

Use two terminals with the same Kubernetes context:

| Terminal | Purpose | Preparation |
| --- | --- | --- |
| A | Sample DACS metrics | Source the `settings.env` path printed in step 2 |
| B | Read and checksum the model | Keep the setup shell, or source the same settings file |

The metrics are **cache-wide**, not per-workload. Keep this dedicated cache
free of other readers and wait for the smoke-test Jobs to finish. Otherwise,
hits cannot reliably be attributed to your test.

Use a **new blob prefix for each cold test**, and keep it unchanged for the
corresponding warm test. Do not overwrite the blobs or clear a shared cache.
A successful mount alone does not establish DACS usage; use the checks below.

### Define the inline metrics check

In **terminal A**, source the settings file, then paste this helper. It queries
`http://localhost:9096/metrics` inside **each of the two CacheServers**; no
Prometheus installation or repository checkout is needed.

```bash
set -euo pipefail

cache_metrics() (
  set -euo pipefail
  pods=$(kubectl get pods -n "$DACS_NAMESPACE" \
    -l "dacs.azure.com/cache-name=$DACS_CACHE" \
    -o 'jsonpath={range .items[*]}{.metadata.name}{" uid="}{.metadata.uid}{" restarts="}{range .status.containerStatuses[*]}{.name}{":"}{.restartCount}{","}{end}{"\n"}{end}' \
    | sort)
  if [[ $(printf '%s\n' "$pods" | awk 'NF { n++ } END { print n+0 }') != 2 ]]; then
    echo "ERROR: Expected two CacheServer pods; check cache readiness." >&2
    exit 1
  fi
  while read -r pod identity; do
    printf '\n%s %s\n' "$pod" "$identity"
    kubectl exec -n "$DACS_NAMESPACE" "$pod" -c cacheserver -- \
      curl --fail --silent --show-error --max-time 10 \
      http://localhost:9096/metrics |
      awk '
        /^cache_server_request_counter\{/ && /request_type="(Download|Upload)"/ {
          print; requests++; next
        }
        /^cache_server_cache_size_bytes(\{|[ \t])/ { print; size++ }
        END {
          if (!requests || !size) {
            print "ERROR: Required cache metrics missing." > "/dev/stderr"
            exit 1
          }
        }'
  done <<< "$pods"
)
```

The output keeps each server's metrics separate. Counters are **cumulative
since server startup**, not per-read results. Save snapshots before and after
each read, compare matching metric labels on each server, then sum the
increases across both servers. Do not compare only one server or treat an
existing nonzero counter as proof of activity during this test.

Each snapshot includes pod names, UIDs, and container restart counts. If these
change between snapshots, or a counter decreases, discard the comparison and
repeat with stable servers. The helper checks scraping and metric presence;
it does not automatically calculate deltas or detect restarts.

### Cold read: populate the cache

In **terminal A**, save a baseline and wait for the command to finish:

```bash
cache_metrics | tee "$DEMO_DIR/cold-before.txt"
```

Then run in **terminal B**:

```bash
kubectl exec -n blob-dacs-demo blob-dacs-reader -- \
  sh -c 'set -eu; time sha256sum "$1"' sh "$MODEL_PATH" \
  | awk -v expected="$MODEL_SHA256" '
      { print; if ($1 == expected) matched = 1 }
      END {
        if (!matched) {
          print "ERROR: Model checksum did not match." > "/dev/stderr"
          exit 1
        }
      }'
```

After the read completes, save another snapshot in **terminal A**:

```bash
cache_metrics | tee "$DEMO_DIR/cold-after.txt"
```

**Checkpoint:** the checksum matches. Across both servers, the counters for
`request_type="Download", status="InvalidTransition"` (misses) and
`request_type="Upload", status="Success"` (cache population) increase, without
request error increases. Some cold-read hits are possible. See
[metric interpretation](#interpret-the-cache-metrics) for the status mappings.
Keep both readers running for asynchronous uploads. If uploads have not yet
been observed, wait and collect another `cold-after.txt` snapshot before
starting the warm read.

### Warm read: reuse data from the second node

After uploads have been observed, save a fresh baseline in **terminal A**:

```bash
cache_metrics | tee "$DEMO_DIR/warm-before.txt"
```

After that command finishes, read the unchanged blob in **terminal B**:

```bash
kubectl exec -n blob-dacs-demo blob-dacs-reader-warm -- \
  sh -c 'set -eu; time sha256sum "$1"' sh "$MODEL_PATH" \
  | awk -v expected="$MODEL_SHA256" '
      { print; if ($1 == expected) matched = 1 }
      END {
        if (!matched) {
          print "ERROR: Model checksum did not match." > "/dev/stderr"
          exit 1
        }
      }'
```

After the read completes, save the ending snapshot in **terminal A**:

```bash
cache_metrics | tee "$DEMO_DIR/warm-after.txt"
```

Compare against `warm-before.txt`, not the cold baseline. Successful
`Download` counters should increase across the two servers. A higher request
hit rate and fewer misses than the cold read are expected once population
completes. Do not require 100% hits: eviction and incomplete population can
cause misses.

### Success checklist

- Both reads complete and match the pinned model checksum.
- The reader pods are on different nodes.
- The sum of increases in `request_type="Download", status="Success"` across
  both servers is positive between the warm snapshots.
- There are no request errors or capability/discovery fallback warnings.

This demonstrates cross-node cache reuse, not that every byte was served by
DACS or that the full file is resident. The `time` output includes checksum
CPU work. Treat read times as supporting evidence only, not a controlled
performance benchmark.

### If the warm result is inconclusive

Do not repeatedly read the same warm mount: node-local caching can hide DACS
activity. Use this bounded recovery procedure:

1. Inspect errors and discovery diagnostics using the next section.
2. If the cold snapshot was taken before population completed, keep the cold
   reader alive, wait up to five minutes, and collect another snapshot to
   check for successful uploads.
3. If there are no upload errors and capacity is sufficient, retry once from
   a fresh reader mount on a supported node that has no active mount of this
   PV. Deleting a pod alone does not guarantee its node's CSI mount is gone.
   Stop the old warm reader and confirm unmount in CSI logs before recreating
   it; rerun the `create_reader` definition if you changed shells.
4. If hits are still absent or misses remain high, stop and investigate cache
   capacity, discovery, and upload failures rather than declaring success.

## 7. Troubleshoot before repeating the test

### Check pod events, DACS health, and CSI logs

```bash
kubectl describe pod blob-dacs-reader -n blob-dacs-demo
kubectl describe pod blob-dacs-reader-warm -n blob-dacs-demo
kubectl get pvc -n blob-dacs-demo
kubectl get pods -n "$DACS_NAMESPACE" \
  -l "dacs.azure.com/cache-name=$DACS_CACHE" -o wide
kubectl logs -n "$DACS_NAMESPACE" \
  -l "dacs.azure.com/cache-name=$DACS_CACHE" \
  -c cacheserver --since=15m --tail=200
kubectl get pods -n kube-system -l app=csi-blob-node -o wide
```

Match the CSI pod's node to the reader's node, then inspect its `blob` container:

```bash
kubectl logs -n kube-system <csi-blob-node-pod-on-reader-node> \
  -c blob --since=15m
```

If the driver cannot verify distributed-cache support, it removes those mount
options and selects local block cache. A warning beginning
`BlobFuse2 distributed-cache support could not be verified` means that mount
is **not using DACS**. Correct the host binary/capability issue and remount.

### Check host BlobFuse2 and proxy diagnostics

BlobFuse2 2.5.6 can mount successfully without discovering any DACS servers and
fall back to Azure Storage. CSI container logs are not the same as host
BlobFuse2 logs. On the affected reader node, use your approved node-access
method and inspect:

```bash
blobfuse2 -v
journalctl -u blobfuse-proxy --since "15 minutes ago" --no-pager
```

Check `/var/log/blobfuse2.log` if that is the configured BlobFuse2 log path,
and `/var/log/syslog` for syslog-based logging. Log routing depends on your
installation. See the [CSI debugging guide](./csi-debug.md) for host log
collection.

| Symptom | Next action |
| --- | --- |
| Pod stays Pending | Inspect pod events, PVC binding, hostname/pool labels, taints, and available node resources. |
| Mount fails | Check host BlobFuse2 version, `fuse2`, read-only settings, Secret namespace/name, shared-key policy, and storage network access. |
| Discovery fails | Verify the discovery service and its endpoints, cluster DNS IP/domain, port 9065, and node-to-cache connectivity. |
| Reads succeed without hits, or misses remain high | Follow [the warm-test recovery procedure](#if-the-warm-result-is-inconclusive). |
| Inline metrics check fails | Check that both CacheServers are Ready, `pods/exec` permissions, container `curl`, metrics port 9096, and the reported error. |
| Measured reads are slow | Check CPU used by checksumming, network throughput, cache resources, and workload access patterns. |

Do not use partial output from a failed metrics check. Resolve the cause and
collect a new baseline. Compare the pod identities and restart counts in both
snapshots; if they change or counters decrease, repeat with stable pods.

## 8. Clean up evaluation resources

Delete only resources created for this walkthrough. Stop all reader pods
before deleting the claim or the cache:

```bash
kubectl delete pod blob-dacs-reader blob-dacs-reader-warm -n blob-dacs-demo
kubectl delete pvc pvc-blob-dacs-demo -n blob-dacs-demo
kubectl delete pv pv-blob-dacs-demo
kubectl delete secret azure-secret -n blob-dacs-demo
kubectl delete namespace blob-dacs-demo
```

The PV uses `Retain`: this does **not** delete the source container or blobs.
The uploaded prefix is printed below. Review it before deleting only that
prefix with your normal storage-management tools:

```bash
printf 'Retained Blob prefix: %s/%s/%s\n' \
  "$STORAGE_ACCOUNT" "$STORAGE_CONTAINER" "$BLOB_PREFIX"
printf 'Retained local working directory: %s\n' "$DEMO_DIR"
```

Remove the local model download and settings when no longer needed. Do not
delete a shared container, other model versions, or unrelated local files.

If DACS was installed only for this evaluation, delete its Cache resource,
then follow the [upstream quickstart cleanup](https://github.com/Azure/dacs/blob/main/docs/quickstart.md#clean-up)
for smoke-test Jobs, the Helm release, and its dedicated namespace:

```bash
kubectl delete cache "$DACS_CACHE" -n "$DACS_NAMESPACE"
```

Leave the managed Blob CSI add-on enabled. For the open-source alternative,
see its conditional cleanup below.

## Advanced details and alternatives

### Open-source Blob CSI driver

Use a cluster where the managed Blob CSI add-on is **not enabled**. Never install
this Helm release alongside the managed add-on. Review the
[open-source driver's identity prerequisites](./install-driver-on-aks.md)
and [Helm installation guidance](../charts/README.md) before proceeding.
If an open-source driver already exists, verify its version and configuration
instead of installing a second release.

Install driver **1.27.11**, explicitly enabling installation of BlobFuse2
**2.5.6** on the nodes:

```bash
helm repo add blob-csi-driver https://kubernetes-sigs.github.io/blob-csi-driver
helm repo update blob-csi-driver
helm install blob-csi-driver blob-csi-driver/blob-csi-driver \
  --namespace kube-system \
  --version 1.27.11 \
  --set node.enableBlobfuseProxy=true \
  --set node.blobfuseProxy.installBlobfuse2=true \
  --set-string node.blobfuseProxy.blobfuse2Version=2.5.6 \
  --wait --timeout 10m
```

These values configure the `install-blobfuse-proxy` init container to install
the pinned binary, independent of the version bundled in the node image.
Return to the driver verification commands in step 3, then continue with the
same DACS, storage, read-only volume, and reader steps.

After completing step 8, uninstall this Helm release **only if you installed
it solely for this evaluation and no other workloads use it**:

```bash
helm uninstall blob-csi-driver -n kube-system
```

### Identity-based storage access

If shared-key authentication is disabled or unsuitable, adapt the
[workload identity example](./workload-identity-static-pv-mount.md) or
[managed identity example](../deploy/example/blobfuse-mi/README.md)
before uploading the model. Configure its identity, role assignments, and
volume authentication instead of the account-key Secret.
Retain `protocol: fuse2`, all read-only settings, and the distributed-cache
mount options.

### Interpret the cache metrics

For each request category below, subtract the baseline from the ending value
for matching metric labels on each server, then sum those increases across
both servers. For example, 30 additional successful downloads on one server
and 50 on the other means DACS served 80 download requests during the read.

| Output | Interpretation |
| --- | --- |
| Downloads served by DACS | `cache_server_request_counter` with `request_type="Download", status="Success"` |
| Downloads not found in DACS | The same counter with `request_type="Download", status="InvalidTransition"` |
| Successful uploads | The same counter with `request_type="Upload", status="Success"` |
| Duplicate uploads | The same counter with `request_type="Upload", status="InvalidTransition"` |
| Download/upload errors | Other statuses for the corresponding request type |
| Cache size at end | Sum of the ending `cache_server_cache_size_bytes` values across both servers, not a counter increase, limit, or per-workload measurement |
| Download request hit rate | `hits / (hits + misses)`; `N/A` when neither is observed |

These mappings follow [DACS's scenario-test checks](https://github.com/Azure/dacs/blob/main/test/pkg/scenario/cache_metrics.go).
Requests are not equivalent to files or bytes, and retries can affect counts.

An increase in successful downloads confirms DACS served requests during the
comparison interval. Misses without successful downloads mean DACS was queried
but did not serve downloads. No increase in either is **inconclusive**, not proof
that DACS is disabled. Follow [the recovery procedure](#if-the-warm-result-is-inconclusive);
for request errors, inspect the logs in step 7.

Duplicate uploads do not add more data. Calculate the download request hit rate
from the summed increases: `hits / (hits + misses)`, excluding errors. It is
undefined if there were no hits or misses, and is not the percentage of
application bytes served or cache capacity utilization.

## References

- [DACS source repository](https://github.com/Azure/dacs)
- [DACS AKS quickstart](https://github.com/Azure/dacs/blob/main/docs/quickstart.md)
- [DACS deployment user guide](https://github.com/Azure/dacs/blob/main/user-guide.md)
- [BlobFuse2 2.5.6 release notes](https://github.com/Azure/azure-storage-fuse/releases/tag/blobfuse2-2.5.6)
- [Blob CSI driver parameters](./driver-parameters.md)
- [Microsoft Phi-1.5 model](https://huggingface.co/microsoft/phi-1_5)
