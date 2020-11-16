## CSI driver troubleshooting guide
### Case#1: volume create/delete issue
 - locate csi driver pod
```console
$ kubectl get po -o wide -n kube-system | grep csi-blob-controller
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blob-controller-56bfddd689-dh5tk       4/4     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-blob-controller-56bfddd689-sl4ll       4/4     Running   0          35s     10.240.0.23    k8s-agentpool-22533604-1
```
 - get csi driver logs
```console
$ kubectl logs csi-blob-controller-56bfddd689-dh5tk -c blob -n kube-system > csi-blob-controller.log
```
> note: there could be multiple controller pods, if there are no helpful logs, try to get logs from other controller pods

### Case#2: volume mount/unmount failed
 - locate csi driver pod and make sure which pod do tha actual volume mount/unmount
```console
$ kubectl get po -o wide -n kube-system | grep csi-blob-node
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blob-node-cvgbs                        3/3     Running   0          7m4s    10.240.0.35    k8s-agentpool-22533604-1
csi-blob-node-dr4s4                        3/3     Running   0          7m4s    10.240.0.4     k8s-agentpool-22533604-0
```

 - get csi driver logs
```console
$ kubectl logs csi-blob-node-cvgbs -c blob -n kube-system > csi-blob-node.log
```

### get blobfuse driver version
```console
# kubectl exec -it csi-blob-node-fmbqw -n kube-system -c blob -- sh
# blobfuse -v
blobfuse 1.2.4
```

### check blobfuse mount on the agent node
```console
# mount | grep blobfuse | uniq
```

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
