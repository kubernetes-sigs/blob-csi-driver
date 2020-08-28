# Get Prometheus metrics from CSI driver

1. Create `csi-blob-controller` service with targetPort `29634`
```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/example/metrics/csi-blob-controller-svc.yaml
```

2. Get ClusterIP of service `csi-blob-controller`
```console
$ kubectl get svc csi-blob-controller -n kube-system
NAME                  TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)     AGE
csi-blob-controller   ClusterIP   10.0.156.8   20.39.0.113   29634/TCP   32m
```

3. Run following command to get cloudprovider_azure metrics
```console
ip=`kubectl get svc csi-blob-controller -n kube-system | grep blob | awk '{print $4}'`
curl http://$ip:29634/metrics | grep cloudprovider_azure | grep -e sum -e count
```
