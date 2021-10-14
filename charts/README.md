# Install CSI driver with Helm 3

## Prerequisites
 - [install Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

### Tips
 - configure with [blobfuse-proxy](../deploy/blobfuse-proxy) to make blobfuse mount still available after driver restart
   - specify `node.enableBlobfuseProxy=true` together with [blobfuse-proxy](../deploy/blobfuse-proxy)
 - make controller only run on master node: `--set controller.runOnMaster=true`
 - enable `fsGroupPolicy` on a k8s 1.20+ cluster: `--set feature.enableFSGroupPolicy=true`
 - set replica of controller as `1`: `--set controller.replicas=1`
 - specify different cloud config secret for the driver:
   - `--set controller.cloudConfigSecretName`
   - `--set controller.cloudConfigSecretNamesapce`
   - `--set node.cloudConfigSecretName`
   - `--set node.cloudConfigSecretNamesapce`
 - switch to `mcr.azk8s.cn` repository in Azure China: `--set image.baseRepo=mcr.azk8s.cn`

## install latest version
```console
helm repo add blob-csi-driver https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/charts
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --set node.enableBlobfuseProxy=true --namespace kube-system
```

### install a specific version
```console
helm repo add blob-csi-driver https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/charts
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --set node.enableBlobfuseProxy=true --namespace kube-system --version v1.6.0
```

## install on Azure Stack
```console
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --set node.enableBlobfuseProxy=true --namespace kube-system --set cloud=AzureStackCloud
```

### install on RedHat/CentOS
```console
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system --set linux.distro=fedora
```

### install driver with customized driver name, deployment name
> only supported from `v1.5.0`+
 - following example would install a driver with name `blob2`
```console
helm install blob2-csi-driver blob-csi-driver/blob-csi-driver --namespace kube-system --set driver.name="blob2.csi.azure.com" --set controller.name="csi-blob2-controller" --set rbac.name=blob2 --set serviceAccount.controller=csi-blob2-controller-sa --set serviceAccount.node=csi-blob2-node-sa --set node.name=csi-blob2-node --set node.livenessProbe.healthPort=29633
```

### search for all available chart versions
```console
helm search repo -l blob-csi-driver
```

## uninstall CSI driver
```console
helm uninstall blob-csi-driver -n kube-system
```

## latest chart configuration

The following table lists the configurable parameters of the latest Azure Blob Storage CSI driver chart and default values.

| Parameter                                             | Description                                           | Default                                                        |
| ----------------------------------------------------- | ----------------------------------------------------- | -------------------------------------------------------------- |
| `driver.name`                                         | alternative driver name                               | `blob.csi.azure.com` |
| `driver.customUserAgent`                              | custom userAgent                                      | `` |
| `driver.userAgentSuffix`                              | userAgent suffix                                      | `OSS-helm` |
| `feature.enableFSGroupPolicy`                         | enable `fsGroupPolicy` on a k8s 1.20+ cluster         | `false`                      |
| `image.baseRepo`                                      | base repository of driver images                      | `mcr.microsoft.com`                      |
| `image.blob.repository`                               | blob-csi-driver docker image                          | `mcr.microsoft.com/k8s/csi/blob-csi`                             |
| `image.blob.tag`                                      | blob-csi-driver docker image tag                      | `latest`                                                         |
| `image.blob.pullPolicy`                               | blob-csi-driver image pull policy                     | `IfNotPresent`                                                   |
| `image.csiProvisioner.repository`                     | csi-provisioner docker image                          | `mcr.microsoft.com/oss/kubernetes-csi/csi-provisioner`           |
| `image.csiProvisioner.tag`                            | csi-provisioner docker image tag                      | `v2.2.2`                                                         |
| `image.csiProvisioner.pullPolicy`                     | csi-provisioner image pull policy                     | `IfNotPresent`                                                   |
| `image.livenessProbe.repository`                      | liveness-probe docker image                           | `mcr.microsoft.com/oss/kubernetes-csi/livenessprobe`             |
| `image.livenessProbe.tag`                             | liveness-probe docker image tag                       | `v2.4.0`                                                         |
| `image.livenessProbe.pullPolicy`                      | liveness-probe image pull policy                      | `IfNotPresent`                                                   |
| `image.nodeDriverRegistrar.repository`                | csi-node-driver-registrar docker image                | `mcr.microsoft.com/oss/kubernetes-csi/csi-node-driver-registrar` |
| `image.nodeDriverRegistrar.tag`                       | csi-node-driver-registrar docker image tag            | `v2.3.0`                                                      |
| `image.nodeDriverRegistrar.pullPolicy`                | csi-node-driver-registrar image pull policy           | `IfNotPresent`                                                   |
| `image.csiResizer.repository`                         | csi-resizer docker image                              | `mcr.microsoft.com/oss/kubernetes-csi/csi-resizer`               |
| `image.csiResizer.tag`                                | csi-resizer docker image tag                          | `v1.3.0`                                                         |
| `image.csiResizer.pullPolicy`                         | csi-resizer image pull policy                         | `IfNotPresent`                                                   |
| `imagePullSecrets`                                    | Specify docker-registry secret names as an array      | [] (does not add image pull secrets to deployed pods)          |
| `serviceAccount.create`                               | whether create service account of csi-blob-controller | `true`                                                           |
| `serviceAccount.controller`                           | name of service account for csi-blob-controller       | `csi-blob-controller-sa`                                  |
| `serviceAccount.node`                                 | name of service account for csi-blob-node             | `csi-blob-node-sa`                                        |
| `rbac.create`                                         | whether create rbac of csi-blob-controller            | `true`                                                           |
| `controller.name`                                     | name of driver deployment                  | `csi-blob-controller`
| `controller.cloudConfigSecretName`                    | cloud config secret name of controller driver               | `azure-cloud-provider`
| `controller.cloudConfigSecretNamespace`               | cloud config secret namespace of controller driver          | `kube-system`
| `controller.replicas`                                 | the replicas of csi-blob-controller                   | `2`                                                              |
| `controller.hostNetwork`                              | `hostNetwork` setting on controller driver(could be disabled if controller does not depend on MSI setting)                            | `true`                                                            | `true`, `false`
| `controller.metricsPort`                              | metrics port of csi-blob-controller                   | `29634`                                                          |
| `controller.livenessProbe.healthPort `                | health check port for liveness probe                   | `29632` |
| `controller.runOnMaster`                              | run controller on master node                         | `true`                                                          |
| `controller.logLevel`                                 | controller driver log level                           | `5`                                                            |
| `controller.resources.csiProvisioner.limits.cpu`      | csi-provisioner cpu limits                            | 100m                                                           |
| `controller.resources.csiProvisioner.limits.memory`   | csi-provisioner memory limits                         | 100Mi                                                          |
| `controller.resources.csiProvisioner.requests.cpu`    | csi-provisioner cpu requests limits                   | 10m                                                            |
| `controller.resources.csiProvisioner.requests.memory` | csi-provisioner memory requests limits                | 20Mi                                                           |
| `controller.resources.livenessProbe.limits.cpu`       | liveness-probe cpu limits                             | 100m                                                           |
| `controller.resources.livenessProbe.limits.memory`    | liveness-probe memory limits                          | 300Mi                                                          |
| `controller.resources.livenessProbe.requests.cpu`     | liveness-probe cpu requests limits                    | 10m                                                            |
| `controller.resources.livenessProbe.requests.memory`  | liveness-probe memory requests limits                 | 20Mi                                                           |
| `controller.resources.blob.limits.cpu`                | blob-csi-driver cpu limits                            | 200m                                                           |
| `controller.resources.blob.limits.memory`             | blob-csi-driver memory limits                         | 200Mi                                                          |
| `controller.resources.blob.requests.cpu`              | blob-csi-driver cpu requests limits                   | 10m                                                            |
| `controller.resources.blob.requests.memory`           | blob-csi-driver memory requests limits                | 20Mi                                                           |
| `controller.resources.csiResizer.limits.cpu`          | csi-resizer cpu limits                                | 100m                                                           |
| `controller.resources.csiResizer.limits.memory`       | csi-resizer memory limits                             | 300Mi                                                          |
| `controller.resources.csiResizer.requests.cpu`        | csi-resizer cpu requests limits                       | 10m                                                            |
| `controller.resources.csiResizer.requests.memory`     | csi-resizer memory requests limits                    | 20Mi                                                           |
| `controller.affinity`                                 | controller pod affinity                               | {}                                                             |
| `controller.nodeSelector`                             | controller pod node selector                          | {}                                                             |
| `controller.tolerations`                              | controller pod tolerations                            | []                                                             |
| `node.name`                                           | name of driver daemonset                              | `csi-blob-node`
| `node.cloudConfigSecretName`                          | cloud config secret name of node driver               | `azure-cloud-provider`
| `node.cloudConfigSecretNamespace`                     | cloud config secret namespace of node driver          | `kube-system`
| `node.maxUnavailable`                                 | `maxUnavailable` value of driver node daemonset       | `1`
| `node.metricsPort`                                    | metrics port of csi-blob-node                         | `29635`                                                          |
| `node.livenessProbe.healthPort `                      | health check port for liveness probe                  | `29633` |
| `node.logLevel`                                       | node driver log level                                 | `5`                                                            |
| `node.enableBlobfuseProxy`                            | node enable blobfuse-proxy                            | `false`                                                          |
| `node.blobfuseCachePath`                              | blobfuse cache path(`tmp-path`)                       | `/mnt`                                                          |
| `node.resources.livenessProbe.limits.cpu`             | liveness-probe cpu limits                             | 100m                                                           |
| `node.resources.livenessProbe.limits.memory`          | liveness-probe memory limits                          | 100Mi                                                          |
| `node.resources.livenessProbe.requests.cpu`           | liveness-probe cpu requests limits                    | 10m                                                            |
| `node.resources.livenessProbe.requests.memory`        | liveness-probe memory requests limits                 | 20Mi                                                           |
| `node.resources.nodeDriverRegistrar.limits.cpu`       | csi-node-driver-registrar cpu limits                  | 100m                                                           |
| `node.resources.nodeDriverRegistrar.limits.memory`    | csi-node-driver-registrar memory limits               | 100Mi                                                          |
| `node.resources.nodeDriverRegistrar.requests.cpu`     | csi-node-driver-registrar cpu requests limits         | 10m                                                            |
| `node.resources.nodeDriverRegistrar.requests.memory`  | csi-node-driver-registrar memory requests limits      | 20Mi                                                           |
| `node.resources.blob.limits.cpu`                      | blob-csi-driver cpu limits                            | `2`                                                            |
| `node.resources.blob.limits.memory`                   | blob-csi-driver memory limits                         | 2100Mi                                                         |
| `node.resources.blob.requests.cpu`                    | blob-csi-driver cpu requests limits                   | 10m                                                            |
| `node.resources.blob.requests.memory`                 | blob-csi-driver memory requests limits                | 20Mi                                                           |
| `node.affinity`                                       | node pod affinity                                     | {}                                                             |
| `node.nodeSelector`                                   | node pod node selector                                | {}                                                             |
| `node.tolerations`                                    | node pod tolerations                                  | []                                                             |
| `linux.kubelet`                                       | configure kubelet directory path on Linux agent node node                  | `/var/lib/kubelet`                                                |
| `linux.distro`                                        | configure ssl certificates for different Linux distribution(available values: `debian`, `fedora`)             | `debian`
| `cloud`                                               | the cloud environment the driver is running on        | `AzurePublicCloud`                                               |
| `podAnnotations`                                      | collection of annotations to add to all the pods      | {}                                                             |
| `podLabels`                                           | collection of labels to add to all the pods           | {}                                                             |
| `priorityClassName`                                   | priority class name to be added to pods               | `system-cluster-critical`                                        |
| `securityContext`                                     | security context to be added to pods                  | {}                                                             |

## troubleshooting
 - Add `--wait -v=5 --debug` in `helm install` command to get detailed error
 - Use `kubectl describe` to acquire more info
