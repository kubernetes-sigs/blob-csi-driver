## End to End Test

## Run E2E tests Locally
### Prerequisite
 - Make sure a kubernetes cluster(with version >= 1.13) is set up and kubeconfig is under `$HOME/.kube/config`. Make sure kubectl is functional.
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

### Run test
```
make e2e-test
```
