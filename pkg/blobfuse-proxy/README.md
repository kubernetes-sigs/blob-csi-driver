# Blobfuse Proxy Development

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

