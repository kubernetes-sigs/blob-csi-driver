# Install CSI driver with Helm 3

## Prerequisites
 - [install Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

### Tips
 - configure with [blobfuse-proxy](../deploy/blobfuse-proxy) to make blobfuse mount still available after driver restart
 > note: [blobfuse-proxy](../deploy/blobfuse-proxy) is only available on **debian** OS based agent node (not available on OpenShift)
   - specify `node.enableBlobfuseProxy=true` together with [blobfuse-proxy](../deploy/blobfuse-proxy)
 - run controller on control plane node: `--set controller.runOnControlPlane=true`
 - set replica of controller as `1`: `--set controller.replicas=1`
 - specify different cloud config secret for the driver:
   - `--set controller.cloudConfigSecretName`
   - `--set controller.cloudConfigSecretNamesapce`
   - `--set node.cloudConfigSecretName`
   - `--set node.cloudConfigSecretNamesapce`
 - switch to `mcr.azk8s.cn` repository in Azure China: `--set image.baseRepo=mcr.azk8s.cn`

### install a specific version
```console
helm repo add blob-csi-driver https://raw.githubusercontent.com/kubernetes-sigs/blob-csi-driver/master/charts
helm install blob-csi-driver blob-csi-driver/blob-csi-driver --set node.enableBlobfuseProxy=true --namespace kube-system --version v1.20.1
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
| `driver.azureGoSDKLogLevel`                           | [Azure go sdk log level](https://github.com/Azure/azure-sdk-for-go/blob/main/documentation/previous-versions-quickstart.md#built-in-basic-requestresponse-logging)  | ``(no logs), `DEBUG`, `INFO`, `WARNING`, `ERROR`, [etc](https://github.com/Azure/go-autorest/blob/50e09bb39af124f28f29ba60efde3fa74a4fe93f/logger/logger.go#L65-L73) |
| `feature.enableFSGroupPolicy`                         | enable `fsGroupPolicy` on a k8s 1.20+ cluster         | `false`                      |
| `feature.enableGetVolumeStats`                        | allow GET_VOLUME_STATS on agent node                  | `false`                      |
| `image.baseRepo`                                      | base repository of driver images                      | `mcr.microsoft.com`                      |
| `image.blob.repository`                               | blob-csi-driver docker image                          | `mcr.microsoft.com/oss/kubernetes-csi/blob-csi`                             |
| `image.blob.tag`                                      | blob-csi-driver docker image tag                      | `latest`                                                         |
| `image.blob.pullPolicy`                               | blob-csi-driver image pull policy                     | `IfNotPresent`                                                   |
| `image.csiProvisioner.repository`                     | csi-provisioner docker image                          | `mcr.microsoft.com/oss/kubernetes-csi/csi-provisioner`           |
| `image.csiProvisioner.tag`                            | csi-provisioner docker image tag                      | `v3.3.0`                                                         |
| `image.csiProvisioner.pullPolicy`                     | csi-provisioner image pull policy                     | `IfNotPresent`                                                   |
| `image.livenessProbe.repository`                      | liveness-probe docker image                           | `mcr.microsoft.com/oss/kubernetes-csi/livenessprobe`             |
| `image.livenessProbe.tag`                             | liveness-probe docker image tag                       | `v2.8.0`                                                         |
| `image.livenessProbe.pullPolicy`                      | liveness-probe image pull policy                      | `IfNotPresent`                                                   |
| `image.nodeDriverRegistrar.repository`                | csi-node-driver-registrar docker image                | `mcr.microsoft.com/oss/kubernetes-csi/csi-node-driver-registrar` |
| `image.nodeDriverRegistrar.tag`                       | csi-node-driver-registrar docker image tag            | `v2.6.2`                                                      |
| `image.nodeDriverRegistrar.pullPolicy`                | csi-node-driver-registrar image pull policy           | `IfNotPresent`                                                   |
| `image.csiResizer.repository`                         | csi-resizer docker image                              | `mcr.microsoft.com/oss/kubernetes-csi/csi-resizer`               |
| `image.csiResizer.tag`                                | csi-resizer docker image tag                          | `v1.6.0`                                                         |
| `image.csiResizer.pullPolicy`                         | csi-resizer image pull policy                         | `IfNotPresent`                                                   |
| `imagePullSecrets`                                    | Specify docker-registry secret names as an array      | [] (does not add image pull secrets to deployed pods)          |
| `cloud`                                               | the cloud environment the driver is running on        | `AzurePublicCloud`                                               |
| `podAnnotations`                                      | collection of annotations to add to all the pods      | {}                                                             |
| `podLabels`                                           | collection of labels to add to all the pods           | {}                                                             |
| `customLabels`                                        | Custom labels to add into metadata                    | `{}`                                                             |
| `priorityClassName`                                   | priority class name to be added to pods               | `system-cluster-critical`                                        |
| `securityContext`                                     | security context to be added to pods                  | {}                                                             |
| `serviceAccount.create`                               | whether create service account of csi-blob-controller | `true`                                                           |
| `serviceAccount.controller`                           | name of service account for csi-blob-controller       | `csi-blob-controller-sa`                                  |
| `serviceAccount.node`                                 | name of service account for csi-blob-node             | `csi-blob-node-sa`                                        |
| `rbac.create`                                         | whether create rbac of csi-blob-controller            | `true`                                                           |
| `controller.name`                                     | name of driver deployment                  | `csi-blob-controller`
| `controller.cloudConfigSecretName`                    | cloud config secret name of controller driver               | `azure-cloud-provider`
| `controller.cloudConfigSecretNamespace`               | cloud config secret namespace of controller driver          | `kube-system`
| `controller.allowEmptyCloudConfig`                    | Whether allow running controller driver without cloud config          | `true`
| `controller.replicas`                                 | replica number of csi-blob-controller                   | `2`                                                              |
| `controller.hostNetwork`                              | `hostNetwork` setting on controller driver(could be disabled if controller does not depend on MSI setting)                            | `true`                                                            | `true`, `false`
| `controller.metricsPort`                              | metrics port of csi-blob-controller                   | `29634`                                                          |
| `controller.livenessProbe.healthPort `                | health check port for liveness probe                   | `29632` |
| `controller.runOnMaster`                              | run controller on master node                         | `false`                                                          |
| `controller.runOnControlPlane`                        | run controller on control plane node                                                          |`false`                                                           |
| `controller.logLevel`                                 | controller driver log level                           | `5`                                                            |
| `controller.resources.csiProvisioner.limits.memory`   | csi-provisioner memory limits                         | 100Mi                                                          |
| `controller.resources.csiProvisioner.requests.cpu`    | csi-provisioner cpu requests                   | 10m                                                            |
| `controller.resources.csiProvisioner.requests.memory` | csi-provisioner memory requests                | 20Mi                                                           |
| `controller.resources.livenessProbe.limits.memory`    | liveness-probe memory limits                          | 300Mi                                                          |
| `controller.resources.livenessProbe.requests.cpu`     | liveness-probe cpu requests                    | 10m                                                            |
| `controller.resources.livenessProbe.requests.memory`  | liveness-probe memory requests                 | 20Mi                                                           |
| `controller.resources.blob.limits.memory`             | blob-csi-driver memory limits                         | 200Mi                                                          |
| `controller.resources.blob.requests.cpu`              | blob-csi-driver cpu requests                   | 10m                                                            |
| `controller.resources.blob.requests.memory`           | blob-csi-driver memory requests                | 20Mi                                                           |
| `controller.resources.csiResizer.limits.memory`       | csi-resizer memory limits                             | 300Mi                                                          |
| `controller.resources.csiResizer.requests.cpu`        | csi-resizer cpu requests                       | 10m                                                            |
| `controller.resources.csiResizer.requests.memory`     | csi-resizer memory requests                    | 20Mi                                                           |
| `controller.affinity`                                 | controller pod affinity                               | {}                                                             |
| `controller.nodeSelector`                             | controller pod node selector                          | {}                                                             |
| `controller.tolerations`                              | controller pod tolerations                            | []                                                             |
| `node.name`                                           | name of driver daemonset                              | `csi-blob-node`
| `node.cloudConfigSecretName`                          | cloud config secret name of node driver               | `azure-cloud-provider`
| `node.cloudConfigSecretNamespace`                     | cloud config secret namespace of node driver          | `kube-system`
| `node.allowEmptyCloudConfig`                          | Whether allow running node driver without cloud config          | `true`
| `node.allowInlineVolumeKeyAccessWithIdentity`         | Whether allow accessing storage account key using cluster identity for inline volume          | `false`
| `node.maxUnavailable`                                 | `maxUnavailable` value of driver node daemonset       | `1`
| `node.livenessProbe.healthPort `                      | health check port for liveness probe                  | `29633` |
| `node.logLevel`                                       | node driver log level                                 | `5`                                                            |
| `node.mountPermissions`                               | mounted folder permissions (only applies for NFS)                 | `0777`
| `node.enableBlobfuseProxy`                            | enable blobfuse-proxy on agent node                           | `false`                                                          |
| `node.blobfuseProxy.installBlobfuse`                  | whether blobfuse should be installed on agent node| `true`                                                          |
| `node.blobfuseProxy.blobfuseVersion`                  | installed blobfuse version on agent node (if the value is empty, it means that the latest version should be installed.) | ``                                                          |
| `node.blobfuseProxy.installBlobfuse2`                 | whether blobfuse2 should be installed on agent node| `true`                                                          |
| `node.blobfuseProxy.blobfuse2Version`                 | installed blobfuse2 version on agent node (if the value is empty, it means that the latest version should be installed.) | ``
| `node.blobfuseProxy.setMaxOpenFileNum`                | whether set max open file num on agent node| `true`                                                          |
| `node.blobfuseProxy.maxOpenFileNum`                   | max open file num on agent node| `9000000`                                                          |
| `node.blobfuseProxy.disableUpdateDB`                  | whether disable updateDB on blobfuse (saving storage account list usage) | `true`                                                          |
| `node.blobfuseCachePath`                              | blobfuse cache path(`tmp-path`)                       | `/mnt`                                                          |
| `node.resources.livenessProbe.limits.memory`          | liveness-probe memory limits                          | 100Mi                                                          |
| `node.resources.livenessProbe.requests.cpu`           | liveness-probe cpu requests                    | 10m                                                            |
| `node.resources.livenessProbe.requests.memory`        | liveness-probe memory requests                 | 20Mi                                                           |
| `node.resources.nodeDriverRegistrar.limits.memory`    | csi-node-driver-registrar memory limits               | 100Mi                                                          |
| `node.resources.nodeDriverRegistrar.requests.cpu`     | csi-node-driver-registrar cpu requests         | 10m                                                            |
| `node.resources.nodeDriverRegistrar.requests.memory`  | csi-node-driver-registrar memory requests      | 20Mi                                                           |
| `node.resources.blob.limits.memory`                   | blob-csi-driver memory limits                         | 2100Mi                                                         |
| `node.resources.blob.requests.cpu`                    | blob-csi-driver cpu requests                   | 10m                                                            |
| `node.resources.blob.requests.memory`                 | blob-csi-driver memory requests                | 20Mi                                                           |
| `node.affinity`                                       | node pod affinity                                     | {}                                                             |
| `node.nodeSelector`                                   | node pod node selector                                | {}                                                             |
| `node.tolerations`                                    | node pod tolerations                                  | []                                                             |
| `linux.kubelet`                                       | configure kubelet directory path on Linux agent node node                  | `/var/lib/kubelet`                                                |
| `linux.distro`                                        | configure ssl certificates for different Linux distribution(available values: `debian`, `fedora`)             | `debian`

## troubleshooting
 - Add `--wait -v=5 --debug` in `helm install` command to get detailed error
 - Use `kubectl describe` to acquire more info
