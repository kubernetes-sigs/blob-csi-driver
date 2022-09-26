# Blobfuse Proxy Development

 - install blobfuse-proxy package, run as a service manually
```console
wget https://github.com/kubernetes-sigs/blob-csi-driver/raw/master/deploy/blobfuse-proxy/v0.1.0/blobfuse-proxy-v0.1.0.deb -O /tmp/blobfuse-proxy-v0.1.0.deb
dpkg -i /tmp/blobfuse-proxy-v0.1.0.deb
mkdir -p /var/lib/kubelet/plugins/blob.csi.azure.com
systemctl enable blobfuse-proxy
systemctl start blobfuse-proxy
```
> blobfuse-proxy start unix socket under `/var/lib/kubelet/plugins/blob.csi.azure.com/blobfuse-proxy.sock` by default

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
