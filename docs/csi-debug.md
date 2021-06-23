## CSI driver troubleshooting guide
### Case#1: volume create/delete issue
 - locate csi driver pod
```console
kubectl get po -o wide -n kube-system | grep csi-blob-controller
```
<pre>
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blob-controller-56bfddd689-dh5tk       4/4     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-blob-controller-56bfddd689-sl4ll       4/4     Running   0          35s     10.240.0.23    k8s-agentpool-22533604-1
</pre>
 - get csi driver logs
```console
kubectl logs csi-blob-controller-56bfddd689-dh5tk -c blob -n kube-system > csi-blob-controller.log
```
> note: there could be multiple controller pods, logs can be taken from all of them simultaneously, also with `follow` (realtime) mode
> `kubectl logs deploy/csi-blob-controller -c blob -f -n kube-system`

### Case#2: volume mount/unmount failed
 - locate csi driver pod and make sure which pod do tha actual volume mount/unmount
```console
kubectl get po -o wide -n kube-system | grep csi-blob-node
```
<pre>
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blob-node-cvgbs                        3/3     Running   0          7m4s    10.240.0.35    k8s-agentpool-22533604-1
csi-blob-node-dr4s4                        3/3     Running   0          7m4s    10.240.0.4     k8s-agentpool-22533604-0
</pre>

 - get csi driver logs
```console
kubectl logs csi-blob-node-cvgbs -c blob -n kube-system > csi-blob-node.log
```
> note: to watch logs in realtime from multiple `csi-blob-node` DaemonSet pods simultaneously, run the command:
> ```console
> kubectl logs daemonset/csi-blob-node -c blob -n kube-system -f
> ```

#### Update driver version quickly by editting driver deployment directly
 - update controller deployment
```console
kubectl edit deployment csi-blob-controller -n kube-system
```
 - update daemonset deployment
```console
kubectl edit ds csi-blob-node -n kube-system
```
change below deployment config, e.g.
```console
        image: mcr.microsoft.com/k8s/csi/blob-csi:v1.5.0
        imagePullPolicy: Always
```

### get blobfuse driver version
```console
kubectl exec -it csi-blob-node-fmbqw -n kube-system -c blob -- sh
blobfuse -v
```
<pre>
blobfuse 1.2.4
</pre>

### check blobfuse mount on the agent node
```console
mount | grep blobfuse | uniq
```

 - Troubleshooting blobfuse mount failure on the agent node
   - collect logs `/var/log/message` if there is blobfuse mount failure, refer to [blobfuse driver troubleshooting](https://github.com/Azure/azure-storage-fuse#logging)

### troubleshooting connection failure on agent node
 - blobfuse

Blobfuse mount will fail due to incorrect storage account name, key or container name, run below commands to check whether blobfuse mount would work on agent node:
```console
mkdir test
export AZURE_STORAGE_ACCOUNT=
export AZURE_STORAGE_ACCESS_KEY=
# only for sovereign cloud
# export AZURE_STORAGE_BLOB_ENDPOINT=accountname.blob.core.chinacloudapi.cn
blobfuse test --container-name=CONTAINER-NAME --tmp-path=/tmp/blobfuse -o allow_other --file-cache-timeout-in-seconds=120
```

 - NFSv3
 
```console
mkdir /tmp/test
mount -t nfs -o sec=sys,vers=3,nolock accountname.blob.core.windows.net:/accountname/container-name /tmp/test
```
