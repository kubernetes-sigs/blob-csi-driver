# blobfuse-proxy
 - supported CSI driver version: v1.1.0 or later version
 - only available on debian based OS

By default, restart csi-blobfuse-node daemonset would make current blobfuse mounts unavailable. When fuse nodeserver restarts on the node, the fuse daemon also restarts, this results in breaking all connections FUSE daemon is maintaining. You could find more details here: [No easy way how to update CSI driver that uses fuse](https://github.com/kubernetes/kubernetes/issues/70013).

This page shows how to run a blobfuse proxy on all agent nodes and this proxy mounts volumes, maintains FUSE connections. 
> Blobfuse proxy receives mount request in a GRPC call and then uses this data to mount and returns the output of the blobfuse command.

### Step#1. Install blobfuse-proxy on debian based agent node
> below daemonset would also install latest [blobfuse](https://github.com/Azure/azure-storage-fuse) version on the node
```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/deploy/blobfuse-proxy/blobfuse-proxy.yaml
```

### Step#2. Install Blob CSI driver with `node.enableBlobfuseProxy=true` helm chart setting
```console
helm repo add blob-csi-driver https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/charts
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system --version v1.4.0 --set node.enableBlobfuseProxy=true
```

#### Troubleshooting
 - Get `blobfuse-proxy` logs on the node
```console
kubectl get po -n kube-system -o wide | grep blobfuse-proxy
csi-blobfuse-proxy-47kpp                    1/1     Running   0          37m
kubectl logs -n kube-system csi-blobfuse-proxy-47kpp
```

#### Development
 - install blobfuse-proxy package, run as a service manually
```console
wget https://github.com/kubernetes-sigs/blob-csi-driver/raw/master/deploy/blobfuse-proxy/v0.1.0/blobfuse-proxy-v0.1.0.deb -O /tmp/blobfuse-proxy-v0.1.0.deb
dpkg -i /tmp/blobfuse-proxy-v0.1.0.deb
mkdir -p /var/lib/kubelet/plugins/blob.csi.azure.com
systemctl enable blobfuse-proxy
systemctl start blobfuse-proxy
```
> blobfuse-proxy start unix socket under `/var/lib/kubelet/blobfuse-proxy.sock` by default

 - make sure all required [Protocol Buffers](https://github.com/protocolbuffers/protobuf) binaries are installed
```console
./hack/install-protoc.sh
```
 - when any change is made to `proto/*.proto` file, run below command to generate
```console
rm pkg/blobfuse-proxy/pb/*.go
protoc --proto_path=pkg/blobfuse-proxy/proto --go-grpc_out=pkg/blobfuse-proxy/pb --go_out=pkg/blobfuse-proxy/pb pkg/blobfuse-proxy/proto/*.proto
```
 - build new blobfuse-proxy binary by running
```console
make blobfuse-proxy
```

 - Generate debian dpkg package
```console
cp _output/blobfuse-proxy ./pkg/blobfuse-proxy/debpackage/usr/bin/blobfuse-proxy
dpkg-deb --build pkg/blobfuse-proxy/debpackage
```

 - Generate redhat/centos package
```console
cp _output/blobfuse-proxy ./pkg/blobfuse-proxy/rpmbuild/SOURCES/blobfuse-proxy
cd ~/rpmbuild/SPECS/
rpmbuild --target noarch -bb utils.spec
```

- Installing blobfuse-proxy package
```console
# On debian based systems
wget https://github.com/kubernetes-sigs/blob-csi-driver/raw/master/deploy/blobfuse-proxy/v0.1.0/blobfuse-proxy-v0.1.0.deb
dpkg -i blobfuse-proxy-v0.1.0.deb

# On redhat/centos based systems
wget https://github.com/kubernetes-sigs/blob-csi-driver/raw/master/deploy/blobfuse-proxy/v0.1.0/blobfuse-proxy-v0.1.0.rpm
rpm -ivh utils-1.0.0-1.noarch.rpm
```
