# Use blobfuse-proxy

By default, restart csi-blobfuse-node daemonsetwould make current blobfuse mounts unavailable. When fuse nodeserver restarts on the node, the fuse daemon also restarts, this results in breaking all connections FUSE daemon is maintaining. You could find more details here: [No easy way how to update CSI driver that uses fuse](https://github.com/kubernetes/kubernetes/issues/70013).

This page shows how to run a blobfuse proxy on all agent nodes and this proxy mounts volumes and maintains FUSE connections. Blobfuse proxy receives mount request in a GRPC call and then uses this data to mount and returns the output of the blobfuse command.

### Prerequisite
 - make sure [blobfuse](https://github.com/Azure/azure-storage-fuse) is already installed on agent node

### Install blobfuse-proxy on debian-based agent node

 - Download blobfuse-proxy package, run as a service
```console
wget https://github.com/kubernetes-sigs/blob-csi-driver/raw/master/deploy/blobfuse-proxy/v0.1.0/blobfuse-proxy-v0.1.0.deb -O /tmp/blobfuse-proxy-v0.1.0.deb
dpkg -i /tmp/blobfuse-proxy-v0.1.0.deb
mkdir -p /var/lib/kubelet/plugins/blob.csi.azure.com
systemctl enable blobfuse-proxy
systemctl start blobfuse-proxy
```
> blobfuse-proxy start unix socket under `/var/lib/kubelet/blobfuse-proxy.sock` by default

#### Troubleshooting
 - Get `blobfuse-proxy` logs
```console
sudo journalctl -u blobfuse-proxy -l > blobfuse-proxy.log
```

#### Development
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
