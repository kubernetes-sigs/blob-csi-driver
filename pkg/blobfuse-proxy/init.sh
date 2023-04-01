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
READ_AHEAD_KB=${READ_AHEAD_KB:-15380}

HOST_CMD="nsenter --mount=/proc/1/ns/mnt"

if [ "${INSTALL_BLOBFUSE}" = "true" ] || [ "${INSTALL_BLOBFUSE2}" = "true" ]
then
  cp /blobfuse-proxy/packages-microsoft-prod.deb /host/etc/
  # when running dpkg -i /etc/packages-microsoft-prod.deb, need to enter y to continue. 
  # refer to https://stackoverflow.com/questions/45349571/how-to-install-deb-with-dpkg-non-interactively
  yes | $HOST_CMD dpkg -i /etc/packages-microsoft-prod.deb && $HOST_CMD apt update

  pkg_list=""
  if [ "${INSTALL_BLOBFUSE}" = "true" ]
  then
    pkg_list="${pkg_list} fuse"
    # install blobfuse with latest version or specific version
    if [ -z "${BLOBFUSE_VERSION}" ]; then
      echo "install blobfuse with latest version"
      pkg_list="${pkg_list} blobfuse"
    else
      pkg_list="${pkg_list} blobfuse=${BLOBFUSE_VERSION}"
    fi
  fi

  if [ "${INSTALL_BLOBFUSE2}" = "true" ]
  then
    release=$($HOST_CMD lsb_release -rs)
    if [ "$release" = "18.04" ]; then
      echo "install fuse for blobfuse2"
      pkg_list="${pkg_list} fuse"
    else
      echo "install fuse3 for blobfuse2, current release is $release"
      pkg_list="${pkg_list} fuse3"
    fi

    # install blobfuse2 with latest version or specific version
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
  rm -rf /host/var/lib/kubelet/plugins/blob.csi.azure.com/blobfuse-proxy.sock
  rm -rf /host/usr/bin/blobfuse-proxy
  cp /blobfuse-proxy/blobfuse-proxy /host/usr/bin/blobfuse-proxy
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

if [ "${SET_READ_AHEAD_SIZE}" = "true" ]
then
  echo "set read ahead size to ${READ_AHEAD_KB}KB"
  AWK_PATH=$(which awk)
  cat > /host/etc/udev/rules.d/99-nfs.rules <<EOF
SUBSYSTEM=="bdi", ACTION=="add", PROGRAM="$AWK_PATH -v bdi=\$kernel 'BEGIN{ret=1} {if (\$4 == bdi){ret=0}} END{exit ret}' /proc/fs/nfsfs/volumes", ATTR{read_ahead_kb}="$READ_AHEAD_KB"
EOF
  $HOST_CMD udevadm control --reload
fi
