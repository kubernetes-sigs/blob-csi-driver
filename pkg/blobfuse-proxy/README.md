### Introduction

Currently there is (not easy way)[https://github.com/kubernetes/kubernetes/issues/70013] to update CSI drivers that use FUSE. Because when a nodeserver restarts, the FUSE daemon also restarts. This results in breaking all this connections the FUSE daemon maintains. So one workaround is to run a fuse proxy
on all the host machines and this proxy mounts volumes and maintains FUSE connections.

Blobfuse proxy receives mount request in a GRPC call and then uses this data to mount and returns the output of the blobfuse command.


### Usage

Run the following command to generate blobfuse proxy
```
make blobfuse-proxy
```
the binary will be generated in `_output/blobfuse-proxy` you can run this binary on linux k8s host machines. But for testing purposes we are using
a daemonset.

### Development

make sure the all required binary are installed
```
./hack/install-proto.sh
```
when every an changes are made to proto/*.proto files, run the below command to generate
```
make clean-proto
make gen-proto
```
After making the required changes you can build the new blobfuse-proxy binary by running
```
make blobfuse-proxy
```

Generating dpkg file in ubuntu
```
make blobfuse-proxy
cp _output/blobfuse-proxy ./pkg/blobfuse-proxy/usr/bin/blobfuse-proxy
dpkg-deb --build pkg/blobfuse-proxy/debpackage
```

#### Installing the agent on debian/ubuntu nodes

Download the blobfuse-proxy `.deb` package file onto host machine and install it using `dpkg -i blobfuse-proxy.deb`
and then start the systemctl service.
```
systemctl enable blobfuse-proxy
systemctl start blobfuse-proxy
```
by default blobfuse-proxy start the unix socket under `/var/lib/kubelet/blobfuse-proxy.sock`
