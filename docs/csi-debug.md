## CSI driver troubleshooting guide

### Case#1: volume create/delete issue
> If you are using [managed CSI driver on AKS](https://docs.microsoft.com/en-us/azure/aks/azure-csi-blob-storage-dynamic), this step does not apply since the driver controller is not visible to the user.

#### Step 1: Find the CSI driver controller pod
> There could be multiple controller pods (only one pod is the leader). If there are no helpful logs, try to get logs from the leader controller pod.
```console
kubectl get po -o wide -n kube-system | grep csi-blob-controller
```
<pre>
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blob-controller-56bfddd689-dh5tk       4/4     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-blob-controller-56bfddd689-sl4ll       4/4     Running   0          35s     10.240.0.23    k8s-agentpool-22533604-1
</pre>

#### Step 2: Get pod description and logs
```console
kubectl describe pod csi-blob-controller-56bfddd689-dh5tk -n kube-system > csi-blob-controller-description.log
kubectl logs csi-blob-controller-56bfddd689-dh5tk -c blob -n kube-system > csi-blob-controller.log
```

#### Common volume create/delete errors

| Error Message | Possible Cause | Resolution |
|---------------|---------------|------------|
| `failed to ensure storage account` | Storage account creation failed due to naming conflict or policy | Check storage account naming rules and Azure Policy constraints |
| `StorageAccountAlreadyTaken` | Storage account name is globally taken | Use `storageAccount` parameter in StorageClass to specify a unique name |
| `AuthorizationFailed` | Insufficient permissions | Ensure the cluster identity has `Storage Account Contributor` role on the resource group |
| `container already exists` | Retry after partial failure | Safe to ignore — the operation is idempotent |

### Case#2: volume mount/unmount failed

#### Step 1: Locate the CSI driver node pod
Identify which node pod handles the actual volume mount/unmount by matching the node where your workload pod is scheduled:
```console
kubectl get po -o wide -n kube-system | grep csi-blob-node
```
<pre>
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blob-node-cvgbs                        3/3     Running   0          7m4s    10.240.0.35    k8s-agentpool-22533604-1
csi-blob-node-dr4s4                        3/3     Running   0          7m4s    10.240.0.4     k8s-agentpool-22533604-0
</pre>

#### Step 2: Get pod description and logs
```console
kubectl describe pod csi-blob-node-cvgbs -n kube-system > csi-blob-node-description.log
kubectl logs csi-blob-node-cvgbs -c blob -n kube-system > csi-blob-node.log
```

> **Tip:** Watch logs in real-time from multiple `csi-blob-node` DaemonSet pods simultaneously:
> ```console
> kubectl logs daemonset/csi-blob-node -c blob -n kube-system -f
> ```

> **Tip:** Get blobfuse-proxy logs on the node:
> ```console
> journalctl -u blobfuse-proxy -l
> ```
> If there are no logs for blobfuse-proxy, check the service status: `systemctl status blobfuse-proxy`

#### Step 3: Verify mounts inside the driver pod

**Check blobfuse mount:**
```console
kubectl exec -it csi-blob-node-cvgbs -c blob -n kube-system -- mount | grep blobfuse
```
<pre>
blobfuse on /var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-efce16db-bf15-4634-b82b-068385019d7c/globalmount type fuse (rw,nosuid,nodev,relatime,user_id=0,group_id=0,allow_other)
blobfuse on /var/lib/kubelet/pods/e73d0984-a253-4203-9e8c-9237ae5c55d5/volumes/kubernetes.io~csi/pvc-efce16db-bf15-4634-b82b-068385019d7c/mount type fuse (rw,relatime,user_id=0,group_id=0,allow_other)
</pre>

**Check NFS mount:**
```console
kubectl exec -it csi-blob-node-cvgbs -n kube-system -c blob -- mount | grep nfs
```
<pre>
accountname.file.core.windows.net:/accountname/pvcn-46c357b2-333b-4c42-8a7f-2133023d6c48 on /var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-46c357b2-333b-4c42-8a7f-2133023d6c48/globalmount type nfs4 (rw,relatime,vers=4.1,rsize=1048576,wsize=1048576,namlen=255,hard,proto=tcp,timeo=600,retrans=2,sec=sys,clientaddr=10.244.0.6,local_lock=none,addr=20.150.29.168)
accountname.file.core.windows.net:/accountname/pvcn-46c357b2-333b-4c42-8a7f-2133023d6c48 on /var/lib/kubelet/pods/7994e352-a4ee-4750-8cb4-db4fcf48543e/volumes/kubernetes.io~csi/pvc-46c357b2-333b-4c42-8a7f-2133023d6c48/mount type nfs4 (rw,relatime,vers=4.1,rsize=1048576,wsize=1048576,namlen=255,hard,proto=tcp,timeo=600,retrans=2,sec=sys,clientaddr=10.244.0.6,local_lock=none,addr=20.150.29.168)
</pre>

#### Common mount errors

| Error Message | Possible Cause | Resolution |
|---------------|---------------|------------|
| `mount failed: exit status 1, fuse: mount failed` | blobfuse binary issue or missing dependencies | Check blobfuse2 version: `blobfuse2 -v` and reinstall if needed |
| `mount error(13): Permission denied` | Invalid storage account key or SAS token | Verify credentials in the Kubernetes secret |
| `mount error(110): Connection timed out` | Network connectivity issue | Check NSG rules, firewall, and private endpoint configuration |
| `Transport endpoint is not connected` | Stale mount — blobfuse process crashed | Restart the pod or run `fusermount -u <mount-path>` on the node |
| `No such file or directory` for NFS | NFS not enabled on storage account | Enable NFS v3 on the storage account and ensure subnet configuration |

### Case#3: Update driver version quickly
You can update the driver version by editing the deployment directly:
```console
# Update controller deployment
kubectl edit deployment csi-blob-controller -n kube-system
# Update daemonset
kubectl edit ds csi-blob-node -n kube-system
```
Change the image and pull policy:
```console
        image: mcr.microsoft.com/k8s/csi/blob-csi:v1.4.0
        imagePullPolicy: Always
```

### Case#4: Troubleshooting on the agent node

#### Get blobfuse driver version
```console
blobfuse2 -v
```
<pre>
blobfuse2 version 2.3.0
</pre>

#### Get OS version
```console
uname -a
```

#### Check blobfuse mount on the node
```console
mount | grep blobfuse | uniq
```

#### Troubleshooting blobfuse mount failure
Collect log files for diagnosis:
- `/var/log/messages`
- `/var/log/syslog`
- `/var/log/blobfuse*.log*`

### Case#5: Troubleshooting connection failure on agent node
> Verify if the mount will work by running the following commands to check if the storage account name, key, and container name are correct.
>
> More details on blobfuse environment variables: https://github.com/Azure/azure-storage-fuse#environment-variables

#### Step 1: Check storage account DNS resolution and connectivity
```console
nslookup accountname.blob.core.windows.net
nc -v -w 2 accountname.blob.core.windows.net 443
```

#### Step 2: Test mount manually

**blobfuse mount with account key:**
```console
mkdir test
export AZURE_STORAGE_ACCOUNT=<account-name>
export AZURE_STORAGE_ACCESS_KEY=<account-key>
# Only for sovereign cloud:
# export AZURE_STORAGE_BLOB_ENDPOINT=accountname.blob.core.chinacloudapi.cn
blobfuse2 test --container-name=CONTAINER-NAME --tmp-path=/tmp/blobfuse -o allow_other --file-cache-timeout-in-seconds=120
```

**blobfuse mount with managed identity:**
```console
mkdir test
export AZURE_STORAGE_ACCOUNT=<account-name>
export AZURE_STORAGE_AUTH_TYPE=MSI
export AZURE_STORAGE_IDENTITY_CLIENT_ID=<client-id>
# Only for sovereign cloud:
# export AZURE_STORAGE_BLOB_ENDPOINT=accountname.blob.core.chinacloudapi.cn
blobfuse2 test --container-name=CONTAINER-NAME --tmp-path=/tmp/blobfuse -o allow_other --file-cache-timeout-in-seconds=120
```

**NFSv3 mount:**
```console
mkdir /tmp/test
mount -v -t nfs -o sec=sys,vers=3,nolock accountname.blob.core.windows.net:/accountname/container-name /tmp/test
```

#### Step 3: Get client-side logs from the node

<details><summary>Collect blobfuse2 logs from the node</summary>

```console
kubectl debug node/{node-name} --image=nginx
# Get blobfuse2 logs
kubectl cp node-debugger-{node-name-xxxx}:/host/var/log/blobfuse2.log /tmp/blobfuse2.log
# Clean up debug pod after collecting logs
kubectl delete po node-debugger-{node-name-xxxx}
```

</details>

### Case#6: Troubleshooting aznfs mount
> Supported from v1.22.2. About aznfs mount helper: https://github.com/Azure/AZNFS-mount/

<details><summary>Check mount point information</summary>

```console
kubectl debug node/node-name --image=nginx
findmnt -t nfs
```

The `SOURCE` of the mount point should have prefix with an IP address rather than domain name, e.g., **10.161.100.100**:/nfs02a796c105814dbebc4e/pvc-ca149059-6872-4d6f-a806-48402648110c.

</details>

<details><summary>Get client-side logs on Linux node</summary>

```console
kubectl debug node/node-name --image=nginx
cat /opt/microsoft/aznfs/data/aznfs.log
```

If IP was migrated successfully, you should find logs like:
1. `IP for nfsxxxxx.blob.core.windows.net changed [1.2.3.4 -> 5.6.7.8].`
2. `Updating mountmap entry [nfsxxxxx.blob.core.windows.net 10.161.100.100 1.2.3.4  -> nfsxxxxx.blob.core.windows.net 10.161.100.100 5.6.7.8]`

</details>

### Tips
 - [Troubleshoot Azure Blob storage mount issues on AKS](http://aka.ms/blobmounterror)
