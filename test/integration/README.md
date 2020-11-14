## Integration Test
Integration test verifies the functionality of CSI driver as a standalone server outside Kubernetes. It exercises the lifecycle of the volume by creating, attaching, staging, mounting volumes and the reverse operations.

## Run Integration Tests Locally
### Prerequisite
 - Make sure `GOPATH` is set and [csc](https://github.com/rexray/gocsi/tree/master/csc) tool is installed under `$GOPATH/bin/csc`
```
export set GOPATH=$HOME/go
go get github.com/rexray/gocsi/csc
cd $GOPATH/src/github.com/rexray/gocsi/csc
make build
export PATH=$PATH:$GOROOT/bin:$GOPATH/bin
```

 - Set Azure credentials by environment variables
 > You could get these variables from `/etc/kubernetes/azure.json` on a kubernetes cluster node
```
# Required environment variables:
export set AZURE_TENANT_ID=
export set AZURE_SUBSCRIPTION_ID=
export set AZURE_CLIENT_ID=
export set AZURE_CLIENT_SECRET=

# Optional environment variables:
# If the the test is not for the public Azure, e.g. Azure China Cloud, then you need to set AZURE_CLOUD_NAME and AZURE_LOCATION.
# For Azure Stack Clound, you need to set AZURE_ENVIRONMENT_FILEPATH for your cloud environment.
# If you have an existing resource group created for the test, then you need to set variable AZURE_RESOURCE_GROUP.
export set AZURE_CLOUD_NAME=
export set AZURE_LOCATION=
export set AZURE_ENVIRONMENT_FILEPATH=
export set AZURE_RESOURCE_GROUP=
```

### Run integration tests
```
make test-integration
```
