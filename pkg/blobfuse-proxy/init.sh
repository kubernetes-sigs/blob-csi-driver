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
INSTALL_BLOBFUSE=${INSTALL_BLOBFUSE:-true}
DISABLE_UPDATEDB=${DISABLE_UPDATEDB:-true}
SET_MAX_OPEN_FILE_NUM=${SET_MAX_OPEN_FILE_NUM:-true}

HOST_CMD="nsenter --mount=/proc/1/ns/mnt"

# install/update blobfuse
if [ "${INSTALL_BLOBFUSE}" = "true" ]
then
  cp /blobfuse-proxy/packages-microsoft-prod.deb /host/etc/
  $HOST_CMD dpkg -i /etc/packages-microsoft-prod.deb && \
  $HOST_CMD apt update && \
  $HOST_CMD apt-get install -y fuse blobfuse2 blobfuse="${BLOBFUSE_VERSION}" && \
  $HOST_CMD rm -f /etc/packages-microsoft-prod.deb
fi

if [ ! -f "/host/usr/bin/blobfuse-proxy" ];then
  echo "copy blobfuse-proxy...."
  cp /blobfuse-proxy/blobfuse-proxy /host/usr/bin/blobfuse-proxy
  chmod 755 /host/usr/bin/blobfuse-proxy
fi

if [ ! -f "/host/usr/lib/systemd/system/blobfuse-proxy.service" ];then
  echo "copy blobfuse-proxy.service...."
  mkdir -p /host/usr/lib/systemd/system
  cp /blobfuse-proxy/blobfuse-proxy.service /host/usr/lib/systemd/system/blobfuse-proxy.service
fi

if [ "${INSTALL_BLOBFUSE_PROXY}" = "true" ]
then
  $HOST_CMD systemctl daemon-reload
  $HOST_CMD systemctl enable blobfuse-proxy.service
  # According to the issue https://github.com/kubernetes-sigs/blob-csi-driver/issues/693,
  # do NOT RESTART blobfuse-proxy, just start it at first time.
  $HOST_CMD systemctl start blobfuse-proxy.service
fi

if [ "${SET_MAX_OPEN_FILE_NUM}" = "true" ]
then
  $HOST_CMD sysctl -w fs.file-max="${MAX_FILE_NUM}"
fi

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
