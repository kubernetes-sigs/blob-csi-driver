# Blobfuse CSI driver development guide

 - Clone repo
```
$ mkdir -p $GOPATH/src/github.com/kubernetes-sigs
$ git clone https://github.com/kubernetes-sigs/blobfuse-csi-driver $GOPATH/src/github.com/kubernetes-sigs
```

 - Build blobfuse driver
```
$ cd $GOPATH/src/github.com/kubernetes-sigs/blobfuse-csi-driver
$ make blobfuse
```

 - Run test
```
$ make test
```

 - Build continer image and push to dockerhub
```
export REGISTRY_NAME=<dockerhub-alias>
make push-latest
```

### Test locally using csc tool
Install `csc` tool according to https://github.com/rexray/gocsi/tree/master/csc:
```
$ mkdir -p $GOPATH/src/github.com
$ cd $GOPATH/src/github.com
$ git clone https://github.com/rexray/gocsi.git
$ cd rexray/gocsi/csc
$ make build
```

#### Start CSI driver locally
```
$ cd $GOPATH/src/github.com/kubernetes-sigs/blobfuse-csi-driver
$ ./_output/blobfuseplugin --endpoint tcp://127.0.0.1:10000 --nodeid CSINode -v=5 &
```
> Before running CSI driver, create "/etc/kubernetes/azure.json" file under testing server(it's better copy `azure.json` file from a k8s cluster with service principle configured correctly) and set `AZURE_CREDENTIAL_FILE` as following:
```
export set AZURE_CREDENTIAL_FILE=/etc/kubernetes/azure.json
```

#### 1. Get plugin info
```
$ csc identity plugin-info --endpoint tcp://127.0.0.1:10000
"blobfuse.csi.azure.com"        "v0.4.0"
```

#### 2. Create an blobfuse volume
```
$ csc controller new --endpoint tcp://127.0.0.1:10000 --cap 1,block CSIVolumeName  --req-bytes 2147483648 --params skuname=Standard_LRS
CSIVolumeID       2147483648      "accountname"="f5713de20cde511e8ba4900" "skuname"="Standard_LRS"
```

#### 3. Mount an blobfuse volume to a user specified directory
```
$ mkdir ~/testmount
$ csc node publish --endpoint tcp://127.0.0.1:10000 --cap 1,block --target-path ~/testmount CSIVolumeID
#f5713de20cde511e8ba4900#pvc-file-dynamic-8ff5d05a-f47c-11e8-9c3a-000d3a00df41
```

#### 4. Unmount blobfuse volume
```
$ csc node unpublish --endpoint tcp://127.0.0.1:10000 --target-path ~/testmount CSIVolumeID
CSIVolumeID
```

#### 5. Delete blobfuse volume
```
$ csc controller del --endpoint tcp://127.0.0.1:10000 CSIVolumeID
CSIVolumeID
```

#### 6. Validate volume capabilities
```
$ csc controller validate-volume-capabilities --endpoint tcp://127.0.0.1:10000 --cap 1,block CSIVolumeID
CSIVolumeID  true
```

#### 7. Get NodeID
```
$ csc node get-info --endpoint tcp://127.0.0.1:10000
CSINode
```

#### 8. Create snapshot
```
$  csc controller create-snapshot
```

#### 9. Delete snapshot
```
$  csc controller delete-snapshot
```
