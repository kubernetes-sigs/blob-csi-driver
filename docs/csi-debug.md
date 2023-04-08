## CSI driver troubleshooting guide
### Case#1: volume create/delete issue
> This step is not applicable if you are using [managed CSI driver on AKS](https://docs.microsoft.com/en-us/azure/aks/azure-csi-blob-storage-dynamic).
 - find csi driver controller pod
> There could be multiple controller pods (only one pod is the leader), if there are no helpful logs, try to get logs from the leader controller pod.
```console
kubectl get po -o wide -n kube-system | grep csi-blob-controller
```
<pre>
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blob-controller-56bfddd689-dh5tk       4/4     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-blob-controller-56bfddd689-sl4ll       4/4     Running   0          35s     10.240.0.23    k8s-agentpool-22533604-1
</pre>

 - get pod description and logs
```console
kubectl describe pod csi-blob-controller-56bfddd689-dh5tk -n kube-system > csi-blob-controller-description.log
kubectl logs csi-blob-controller-56bfddd689-dh5tk -c blob -n kube-system > csi-blob-controller.log
```

### Case#2: volume mount/unmount failed
 - locate csi driver pod and make sure which pod does the actual volume mount/unmount
```console
kubectl get po -o wide -n kube-system | grep csi-blob-node
```
<pre>
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blob-node-cvgbs                        3/3     Running   0          7m4s    10.240.0.35    k8s-agentpool-22533604-1
csi-blob-node-dr4s4                        3/3     Running   0          7m4s    10.240.0.4     k8s-agentpool-22533604-0
</pre>

 - get pod description and logs
```console
kubectl describe pod csi-blob-node-cvgbs -n kube-system > csi-blob-node-description.log
kubectl logs csi-blob-node-cvgbs -c blob -n kube-system > csi-blob-node.log
```
> note: to watch logs in realtime from multiple `csi-blob-node` DaemonSet pods simultaneously, run the command:
> ```console
> kubectl logs daemonset/csi-blob-node -c blob -n kube-system -f
> ```
> get blobfuse-proxy logs on the node
> ```console
> journalctl -u blobfuse-proxy -l
> ```

 - check blobfuse mount inside driver
```console
kubectl exec -it csi-blob-node-9vl9t -c blob -n kube-system -- mount | grep blobfuse
```
<pre>
blobfuse on /var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-efce16db-bf15-4634-b82b-068385019d7c/globalmount type fuse (rw,nosuid,nodev,relatime,user_id=0,group_id=0,allow_other)
blobfuse on /var/lib/kubelet/pods/e73d0984-a253-4203-9e8c-9237ae5c55d5/volumes/kubernetes.io~csi/pvc-efce16db-bf15-4634-b82b-068385019d7c/mount type fuse (rw,relatime,user_id=0,group_id=0,allow_other)
</pre>

 - check nfs mount inside driver
```console
kubectl exec -it csi-blob-node-9vl9t -n kube-system -c blob -- mount | grep nfs
```
<pre>
accountname.file.core.windows.net:/accountname/pvcn-46c357b2-333b-4c42-8a7f-2133023d6c48 on /var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-46c357b2-333b-4c42-8a7f-2133023d6c48/globalmount type nfs4 (rw,relatime,vers=4.1,rsize=1048576,wsize=1048576,namlen=255,hard,proto=tcp,timeo=600,retrans=2,sec=sys,clientaddr=10.244.0.6,local_lock=none,addr=20.150.29.168)
accountname.file.core.windows.net:/accountname/pvcn-46c357b2-333b-4c42-8a7f-2133023d6c48 on /var/lib/kubelet/pods/7994e352-a4ee-4750-8cb4-db4fcf48543e/volumes/kubernetes.io~csi/pvc-46c357b2-333b-4c42-8a7f-2133023d6c48/mount type nfs4 (rw,relatime,vers=4.1,rsize=1048576,wsize=1048576,namlen=255,hard,proto=tcp,timeo=600,retrans=2,sec=sys,clientaddr=10.244.0.6,local_lock=none,addr=20.150.29.168)
</pre>

#### Update driver version quickly by editing driver deployment directly
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
        image: mcr.microsoft.com/k8s/csi/blob-csi:v1.4.0
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

To check if blobfuse mount would work on the agent node, run the following commands to verify that the storage account name, key, and container name are correct. If any of these are incorrect, the blobfuse mount will fail:
```console
mkdir test
export AZURE_STORAGE_ACCOUNT=
export AZURE_STORAGE_ACCESS_KEY=
# only for sovereign cloud
# export AZURE_STORAGE_BLOB_ENDPOINT=accountname.blob.core.chinacloudapi.cn
blobfuse test --container-name=CONTAINER-NAME --tmp-path=/tmp/blobfuse -o allow_other --file-cache-timeout-in-seconds=120
```
> You can find more detailed information about environment variables at https://github.com/Azure/azure-storage-fuse#environment-variables.

 - NFSv3
 
```console
mkdir /tmp/test
mount -v -t nfs -o sec=sys,vers=3,nolock accountname.blob.core.windows.net:/accountname/container-name /tmp/test
```

<details><summary>
Get client-side logs on AKS Linux node if there is mount error 
</summary>

```console
# get ama-logs pod which is running on the AKS Linux node
kubectl get po -n kube-system -o wide | grep ama-logs
# get blobfuse logs
kubectl -n kube-system cp ama-logs-xxxx:/var/log/blobfuse.log /tmp/blobfuse.log
# get blobfuse2 logs
kubectl -n kube-system cp ama-logs-xxxx:/var/log/blobfuse2.log /tmp/blobfuse2.log
```

</details>

<details><summary>
Get client-side logs on Linux node if there is mount error 
</summary>

```console
kubectl debug node/node-name --image=nginx
# get blobfuse logs
kubectl cp node-debugger-node-name-xxxx:/host/var/log/blobfuse.log /tmp/blobfuse.log
# get blobfuse2 logs
kubectl cp node-debugger-node-name-xxxx:/host/var/log/blobfuse2.log /tmp/blobfuse2.log
#after log collected, delete the debug pod by:
kubectl delete po node-debugger-node-name-xxxx
```
 
</details>

### Tips
 - [Troubleshoot Azure Blob storage mount issues on AKS](http://aka.ms/blobmounterror)
