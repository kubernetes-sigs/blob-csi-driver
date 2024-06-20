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

# install blobfuse/blobfuse2
if [ "${DISTRIBUTION}" != "ubuntu" ]
then
  echo "skip install blobfuse/blobfuse2 for ${DISTRIBUTION}...."
elif [ "${ARCH}" = "aarch64" ]
then
  echo "skip install blobfuse/blobfuse2 for arm64...."
elif [ "${INSTALL_BLOBFUSE}" = "true" ] || [ "${INSTALL_BLOBFUSE2}" = "true" ]
then
  echo "start to install blobfuse/blobfuse2...."

  release=$($HOST_CMD lsb_release -rs)
  echo "Ubuntu release: $release"
  
  if [ "$(expr "$release" \< "22.04")" -eq 1 ]
  then
    cp /blobfuse-proxy/packages-microsoft-prod-18.04.deb /host/etc/packages-microsoft-prod.deb
  else
    cp /blobfuse-proxy/packages-microsoft-prod-22.04.deb /host/etc/packages-microsoft-prod.deb
  fi
  # when running dpkg -i /etc/packages-microsoft-prod.deb, need to enter y to continue. 
  # refer to https://stackoverflow.com/questions/45349571/how-to-install-deb-with-dpkg-non-interactively
  yes | $HOST_CMD dpkg -i /etc/packages-microsoft-prod.deb && $HOST_CMD apt update

  pkg_list=""
  # blobfuse
  if [ "${INSTALL_BLOBFUSE}" = "true" ] && [ "$(expr "$release" \< "22.04")" -eq 1 ]
  then
    pkg_list="${pkg_list} fuse"
    if [ -z "${BLOBFUSE_VERSION}" ]; then
      echo "install blobfuse with latest version"
      pkg_list="${pkg_list} blobfuse"
    else
      pkg_list="${pkg_list} blobfuse=${BLOBFUSE_VERSION}"
    fi
  fi

  # blobfuse2
  if [ "${INSTALL_BLOBFUSE2}" = "true" ]
  then
    if [ "$(expr "$release" \< "22.04")" -eq 1 ]; then
      echo "install fuse for blobfuse2"
      pkg_list="${pkg_list} fuse"
    else
      echo "install fuse3 for blobfuse2, current release is $release"
      pkg_list="${pkg_list} fuse3"
    fi

    if [ -z "${BLOBFUSE2_VERSION}" ]; then
      echo "install blobfuse2 with latest version"
      pkg_list="${pkg_list} blobfuse2"
    else
      pkg_list="${pkg_list} blobfuse2=${BLOBFUSE2_VERSION}"
    fi
  fi

  echo "begin to install ${pkg_list}"
  $HOST_CMD apt-get install -y $pkg_list
  $HOST_CMD rm -f /etc/packages-microsoft-prod.deb
fi

# install blobfuse-proxy
updateBlobfuseProxy="true"
if [ -f "/host/usr/bin/blobfuse-proxy" ];then
  old=$(sha256sum /host/usr/bin/blobfuse-proxy | awk '{print $1}')
  new=$(sha256sum /blobfuse-proxy/blobfuse-proxy | awk '{print $1}')
  if [ "$old" = "$new" ];then
    updateBlobfuseProxy="false"
    echo "no need to update blobfuse-proxy"
  fi
fi
if [ "$updateBlobfuseProxy" = "true" ];then
  echo "copy blobfuse-proxy...."
  rm -rf /host/"$KUBELET_PATH"/plugins/blob.csi.azure.com/blobfuse-proxy.sock
  cp /blobfuse-proxy/blobfuse-proxy /host/usr/bin/blobfuse-proxy --force
  chmod 755 /host/usr/bin/blobfuse-proxy
fi

updateService="true"
if [ -f "/host/usr/lib/systemd/system/blobfuse-proxy.service" ];then
  old=$(sha256sum /host/usr/lib/systemd/system/blobfuse-proxy.service | awk '{print $1}')
  new=$(sha256sum /blobfuse-proxy/blobfuse-proxy.service | awk '{print $1}')
  if [ "$old" = "$new" ];then
      updateService="false"
      echo "no need to update blobfuse-proxy.service"
  fi
fi
if [ "$updateService" = "true" ];then
  echo "copy blobfuse-proxy.service...."
  mkdir -p /host/usr/lib/systemd/system
  cp /blobfuse-proxy/blobfuse-proxy.service /host/usr/lib/systemd/system/blobfuse-proxy.service
fi

if [ "${INSTALL_BLOBFUSE_PROXY}" = "true" ];then
  if [ "$updateBlobfuseProxy" = "true" ] || [ "$updateService" = "true" ];then
    echo "start blobfuse-proxy...."
    $HOST_CMD systemctl daemon-reload
    $HOST_CMD systemctl enable blobfuse-proxy.service
    $HOST_CMD systemctl restart blobfuse-proxy.service
  fi
fi
