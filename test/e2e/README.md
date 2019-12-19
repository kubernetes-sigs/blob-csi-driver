## End to End Test

## Run E2E tests Locally

### Prerequisite

 - Make sure a kubernetes cluster(with version >= 1.13) is set up and kubeconfig is under `$HOME/.kube/config`
 - Copy out `/etc/kubernetes/azure.json` under one agent node to local machine
 - Set `AZURE_CREDENTIAL_FILE` env variable to path of file copied from `/etc/kubernetes/azure.json`
 > For AKS cluster, need to modify `resourceGroup` to the node resource group name inside `/etc/kubernetes/azure.json`

### Run test

```
make e2e-test
```
