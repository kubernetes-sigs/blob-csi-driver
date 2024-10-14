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

ARG ARCH=amd64

FROM registry.k8s.io/build-image/debian-base:bookworm-v1.0.4 AS base

FROM base AS builder

ARG ARCH

RUN apt update \
    && apt install -y curl

# install aznfs
ARG aznfsVer=2.0.7
RUN if [ "$ARCH" = "arm64" ]; then \
      curl -Ls https://github.com/Azure/AZNFS-mount/releases/download/${aznfsVer}/aznfs-${aznfsVer}-1.arm64.tar.gz | tar xvzf - -C / --keep-directory-symlink; \
    else \
      curl -Ls https://github.com/Azure/AZNFS-mount/releases/download/${aznfsVer}/aznfs-${aznfsVer}-1.x86_64.tar.gz | tar xvzf - -C / --keep-directory-symlink; \
    fi

# install azcopy
RUN curl -Ls https://azcopyvnext.azureedge.net/releases/release-10.26.0-20240731/azcopy_linux_${ARCH}_10.26.0.tar.gz \
        | tar xvzf - --strip-components=1 -C /usr/local/bin/ --wildcards "*/azcopy"

# download blobfuse deb
RUN mkdir /blobfuse-proxy/
RUN curl -Lso /blobfuse-proxy/packages-microsoft-prod-18.04.deb https://packages.microsoft.com/config/ubuntu/18.04/packages-microsoft-prod.deb
RUN curl -Lso /blobfuse-proxy/packages-microsoft-prod-22.04.deb https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb
FROM base

ARG ARCH
ARG binary=./_output/${ARCH}/blobplugin
COPY ${binary} /blobplugin

COPY --from=builder --chown=root:root /opt/microsoft/aznfs /opt/microsoft/aznfs
COPY --from=builder --chown=root:root /sbin/mount.aznfs /sbin/mount.aznfs
COPY --from=builder --chown=root:root /usr/sbin/aznfswatchdog /usr/sbin/aznfswatchdog
COPY --from=builder --chown=root:root /usr/local/bin/azcopy /usr/local/bin/azcopy
COPY --from=builder --chown=root:root /blobfuse-proxy /blobfuse-proxy

# packages that are only needed by aznfs: procps conntrack iptables bind9-host iproute2 bash netcat sysvinit-utils.
RUN apt update && apt upgrade -y && apt-mark unhold libcap2 && clean-install ca-certificates uuid-dev util-linux mount udev e2fsprogs nfs-common netbase procps conntrack iptables bind9-host iproute2 bash netcat-traditional sysvinit-utils kmod

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

RUN if [ "$ARCH" = "amd64" ] ; then \
    clean-install libcurl4-gnutls-dev \
    && dpkg -i /blobfuse-proxy/packages-microsoft-prod-18.04.deb \
    && rm /blobfuse-proxy/packages-microsoft-prod-18.04.deb \
    && apt update \
    && apt install -y blobfuse blobfuse2 fuse; fi

RUN if [ "$ARCH" = "arm64" ] ; then \
    dpkg -i /blobfuse-proxy/packages-microsoft-prod-22.04.deb \
    && apt update \
    && apt install -y fuse3 blobfuse2; fi

LABEL maintainers="andyzhangx"
LABEL description="Azure Blob Storage CSI driver"

ENTRYPOINT ["/blobplugin"]
