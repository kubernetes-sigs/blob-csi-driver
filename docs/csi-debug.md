## CSI driver debug tips
### Condition#1: volume create/delete issue
 - locate csi driver pod
```sh
$ kubectl get po -o wide -n kube-system | grep csi-blobfuse-controller
NAME                                           READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blobfuse-controller-56bfddd689-dh5tk       5/5     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
```
 - get csi driver logs
```sh
$ kubectl logs `kubectl get po -n kube-system | grep csi-blobfuse-controller | cut -d ' ' -f1` -c blobfuse -n kube-system > csi-blobfuse-controller.log
```
> note: there could be multiple controller pods, if there is no useful logs, switch to get logs from other controller pods

### Condition#2: volume mount/unmount failed
 - locate csi driver pod and make sure which pod do tha actual volume mount/unmount
```
$ kubectl get po -o wide -n kube-system | grep csi-blobfuse-node
NAME                                           READY   STATUS    RESTARTS   AGE     IP             NODE
csi-blobfuse-node-cvgbs                        3/3     Running   0          7m4s    10.240.0.35    k8s-agentpool-22533604-1
csi-blobfuse-node-dr4s4                        3/3     Running   0          7m4s    10.240.0.4     k8s-agentpool-22533604-0
```

 - get csi driver logs
```sh
$ kubectl logs csi-blobfuse-node-cvgbs -c blobfuse -n kube-system > csi-blobfuse-node.log
```
