#!/bin/sh

# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -xe

INSTALL_BLOBFUSE_PROXY=${INSTALL_BLOBFUSE_PROXY:-true}
DISABLE_UPDATEDB=${DISABLE_UPDATEDB:-true}
SET_MAX_OPEN_FILE_NUM=${SET_MAX_OPEN_FILE_NUM:-true}
SET_READ_AHEAD_SIZE=${SET_READ_AHEAD_SIZE:-true}
MIGRATE_K8S_REPO=${MIGRATE_K8S_REPO:-false}
READ_AHEAD_KB=${READ_AHEAD_KB:-15380}
KUBELET_PATH=${KUBELET_PATH:-/var/lib/kubelet}
ALLOW_PACKAGE_INSTALL_DOWNGRADE=${ALLOW_PACKAGE_INSTALL_DOWNGRADE:-true}

if [ "$KUBELET_PATH" != "/var/lib/kubelet" ];then
  echo "kubelet path is $KUBELET_PATH, update blobfuse-proxy.service...."
  sed -i "s#--blobfuse-proxy-endpoint[^ ]*#--blobfuse-proxy-endpoint=unix:/${KUBELET_PATH}/plugins/blob.csi.azure.com/blobfuse-proxy.sock#" /blobfuse-proxy/blobfuse-proxy.service
  echo "blobfuse-proxy endpoint is updated to unix:/$KUBELET_PATH/plugins/blob.csi.azure.com/blobfuse-proxy.sock"
fi 

HOST_CMD="nsenter --mount=/proc/1/ns/mnt"

DISTRIBUTION=$($HOST_CMD cat /etc/os-release | grep ^ID= | cut -d'=' -f2 | tr -d '"')
ARCH=$($HOST_CMD uname -m)
echo "Linux distribution: $DISTRIBUTION, Arch: $ARCH"

# install blobfuse-proxy and blobfuse/blofuse2 if needed
case "${DISTRIBUTION}" in
  "rhcos" | "cos")
    . ./blobfuse-proxy/install-proxy-rhcos.sh
    ;;
  *)
    . ./blobfuse-proxy/install-proxy.sh
    ;;
esac

# set max open file num
if [ "${SET_MAX_OPEN_FILE_NUM}" = "true" ]
then
  $HOST_CMD sysctl -w fs.file-max="${MAX_FILE_NUM}"
fi

# disable updatedb
updateDBConfigPath="/host/etc/updatedb.conf"
if [ "${DISABLE_UPDATEDB}" = "true" ] && [ -f ${updateDBConfigPath} ]
then
  echo "before changing ${updateDBConfigPath}:"
  cat ${updateDBConfigPath}
  sed -i 's/PRUNEPATHS="\/tmp/PRUNEPATHS="\/mnt \/var\/lib\/kubelet \/tmp/g' ${updateDBConfigPath}
  sed -i 's/PRUNEFS="NFS/PRUNEFS="fuse blobfuse NFS/g' ${updateDBConfigPath}
  echo "after change:"
  cat ${updateDBConfigPath}
fi

# set read ahead size
if [ "${SET_READ_AHEAD_SIZE}" = "true" ]
then
  echo "set read ahead size to ${READ_AHEAD_KB}KB"
  AWK_PATH=$(which awk)
  cat > /host/etc/udev/rules.d/99-nfs.rules <<EOF
SUBSYSTEM=="bdi", ACTION=="add", PROGRAM="$AWK_PATH -v bdi=\$kernel 'BEGIN{ret=1} {if (\$4 == bdi){ret=0}} END{exit ret}' /proc/fs/nfsfs/volumes", ATTR{read_ahead_kb}="$READ_AHEAD_KB"
EOF
  $HOST_CMD udevadm control --reload
fi
