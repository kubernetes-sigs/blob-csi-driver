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

FROM registry.k8s.io/build-image/debian-base:bookworm-v1.0.3

ARG ARCH=amd64
ARG binary=./_output/${ARCH}/blobplugin
COPY ${binary} /blobplugin

RUN mkdir /blobfuse-proxy/

COPY ./pkg/blobfuse-proxy/init.sh /blobfuse-proxy/
COPY ./pkg/blobfuse-proxy/install-proxy.sh /blobfuse-proxy/
COPY ./pkg/blobfuse-proxy/install-proxy-rhcos.sh /blobfuse-proxy/
COPY ./pkg/blobfuse-proxy/blobfuse-proxy.service /blobfuse-proxy/
COPY ./_output/${ARCH}/blobfuse-proxy /blobfuse-proxy/

RUN chmod +x /blobfuse-proxy/init.sh && \
 chmod +x /blobfuse-proxy/install-proxy-rhcos.sh && \
 chmod +x /blobfuse-proxy/install-proxy.sh && \
 chmod +x /blobfuse-proxy/blobfuse-proxy.service && \
 chmod +x /blobfuse-proxy/blobfuse-proxy

# packages that are only needed by aznfs: procps conntrack iptables bind9-host iproute2 bash netcat sysvinit-utils.
RUN apt update && apt upgrade -y && apt-mark unhold libcap2 && clean-install ca-certificates uuid-dev util-linux mount udev wget e2fsprogs nfs-common netbase procps conntrack iptables bind9-host iproute2 bash netcat-traditional sysvinit-utils kmod

# install aznfs
ARG aznfsVer=2.0.4
RUN if [ "$ARCH" = "amd64" ] ; then \
  wget -O aznfs.tar.gz https://github.com/Azure/AZNFS-mount/releases/download/${aznfsVer}/aznfs-${aznfsVer}-1.x86_64.tar.gz; \
else \
  wget -O aznfs.tar.gz https://github.com/Azure/AZNFS-mount/releases/download/${aznfsVer}/aznfs-${aznfsVer}-1.arm64.tar.gz;fi
RUN tar xvzf aznfs.tar.gz -C / --keep-directory-symlink && rm aznfs.tar.gz

# install azcopy
ARG azcopyURL=https://azcopyvnext.azureedge.net/releases/release-10.25.0-20240522/azcopy_linux_amd64_10.25.0.tar.gz
RUN if [ "$ARCH" == "arm64" ] ; then \
  azcopyURL=https://azcopyvnext.azureedge.net/releases/release-10.25.0-20240522/azcopy_linux_arm64_10.25.0.tar.gz; fi
RUN wget -O azcopy.tar.gz ${azcopyURL} && \
  tar xvzf azcopy.tar.gz -C . && rm azcopy.tar.gz && \
  mv ./azcopy_linux_$ARCH_*/azcopy /usr/local/bin/azcopy && \
  rm -rf ./azcopy_linux_$ARCH_*
RUN chmod +x /usr/local/bin/azcopy

RUN if [ "$ARCH" = "amd64" ] ; then \
  clean-install libcurl4-gnutls-dev && \
  wget -O /blobfuse-proxy/packages-microsoft-prod-22.04.deb https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb && \
  wget -O /blobfuse-proxy/packages-microsoft-prod-18.04.deb https://packages.microsoft.com/config/ubuntu/18.04/packages-microsoft-prod.deb && \
  dpkg -i /blobfuse-proxy/packages-microsoft-prod-18.04.deb && apt update && apt install blobfuse blobfuse2 fuse -y && apt remove wget -y; fi
LABEL maintainers="andyzhangx"
LABEL description="Azure Blob Storage CSI driver"

ENTRYPOINT ["/blobplugin"]
