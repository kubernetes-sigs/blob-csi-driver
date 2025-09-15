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

BIN_PATH=${BIN_PATH:-/usr/local/bin}
if [ "${DISTRIBUTION}" = "cos" ]; then
  echo "set BIN_PATH to /home/kubernetes/bin"
  BIN_PATH="/home/kubernetes/bin"
fi
if [ "${DISTRIBUTION}" = "gardenlinux" ]; then
  echo "set BIN_PATH to /var/bin"
  BIN_PATH="/var/bin"
fi
if [ "${CUSTOM_BIN_PATH}" != "" ]; then
  echo "set BIN_PATH to ${CUSTOM_BIN_PATH}"
  BIN_PATH="${CUSTOM_BIN_PATH}"
fi

#copy blobfuse2 binary to BIN_PATH/blobfuse2
updateBlobfuse2="true"
if [ "${INSTALL_BLOBFUSE}" = "true" ] || [ "${INSTALL_BLOBFUSE2}" = "true" ]
then
  if [ -f "/host${BIN_PATH}/blobfuse2" ];then
    old=$(sha256sum /host${BIN_PATH}/blobfuse2 | awk '{print $1}')
    new=$(sha256sum /usr/bin/blobfuse2 | awk '{print $1}')
    if [ "$old" = "$new" ];then
      updateBlobfuse2="false"
      echo "no need to update blobfuse2 since no change"
    fi
  fi
else
  echo "no need to install blobfuse2"
  updateBlobfuse2="false"
fi
if [ "$updateBlobfuse2" = "true" ];then
  echo "copy blobfuse2...."
  cp /usr/bin/blobfuse2 /host${BIN_PATH}/blobfuse2 --force
  chmod 755 /host${BIN_PATH}/blobfuse2
fi

if [ "${INSTALL_BLOBFUSE_PROXY}" = "true" ];then
  # install blobfuse-proxy
  updateBlobfuseProxy="true"
  if [ -f "/host${BIN_PATH}/blobfuse-proxy" ];then
    old=$(sha256sum /host${BIN_PATH}/blobfuse-proxy | awk '{print $1}')
    new=$(sha256sum /blobfuse-proxy/blobfuse-proxy | awk '{print $1}')
    if [ "$old" = "$new" ];then
      updateBlobfuseProxy="false"
      echo "no need to update blobfuse-proxy"
    fi
  fi
  if [ "$updateBlobfuseProxy" = "true" ];then
    echo "copy blobfuse-proxy...."
    rm -rf /host/${KUBELET_PATH}/plugins/blob.csi.azure.com/blobfuse-proxy.sock
    cp /blobfuse-proxy/blobfuse-proxy /host${BIN_PATH}/blobfuse-proxy --force
    chmod 755 /host${BIN_PATH}/blobfuse-proxy
  fi

  updateService="true"
  echo "change from /usr/bin/blobfuse-proxy to ${BIN_PATH}/blobfuse-proxy in blobfuse-proxy.service"
  sed -i "s|/usr/bin/blobfuse-proxy|${BIN_PATH}/blobfuse-proxy|g" /blobfuse-proxy/blobfuse-proxy.service
  if [ "${BIN_PATH}" != "/usr/local/bin" ]; then
    echo "add Environment=\"PATH=${BIN_PATH}:$PATH\" in blobfuse-proxy.service"
    sed -i "/Delegate=yes/a Environment=\"PATH=${BIN_PATH}:\$PATH\"" /blobfuse-proxy/blobfuse-proxy.service
  fi
  if [ -f "/host/etc/systemd/system/blobfuse-proxy.service" ];then
    old=$(sha256sum /host/etc/systemd/system/blobfuse-proxy.service | awk '{print $1}')
    new=$(sha256sum /blobfuse-proxy/blobfuse-proxy.service | awk '{print $1}')
    if [ "$old" = "$new" ];then
        updateService="false"
        echo "no need to update blobfuse-proxy.service"
    fi
  fi
  if [ "$updateService" = "true" ];then
    echo "copy blobfuse-proxy.service...."
    mkdir -p /host/etc/systemd/system/
    cp /blobfuse-proxy/blobfuse-proxy.service /host/etc/systemd/system/blobfuse-proxy.service
  fi

  $HOST_CMD systemctl daemon-reload
  $HOST_CMD systemctl enable blobfuse-proxy.service
  if [ "$updateBlobfuseProxy" = "true" ] || [ "$updateService" = "true" ];then
    echo "restart blobfuse-proxy...."
    $HOST_CMD systemctl restart blobfuse-proxy.service
  else
    echo "start blobfuse-proxy...."
    $HOST_CMD systemctl start blobfuse-proxy.service
  fi
fi
