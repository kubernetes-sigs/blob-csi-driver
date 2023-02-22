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

FROM registry.k8s.io/build-image/debian-base:bullseye-v1.4.3

ARG ARCH=amd64
ARG binary=./_output/${ARCH}/blobplugin
COPY ${binary} /blobplugin

RUN mkdir /blobfuse-proxy/

COPY ./pkg/blobfuse-proxy/init.sh /blobfuse-proxy/
COPY ./pkg/blobfuse-proxy/blobfuse-proxy.service /blobfuse-proxy/
COPY ./_output/${ARCH}/blobfuse-proxy /blobfuse-proxy/

RUN chmod +x /blobfuse-proxy/init.sh && \
 chmod +x /blobfuse-proxy/blobfuse-proxy.service && \
 chmod +x /blobfuse-proxy/blobfuse-proxy

RUN apt update && apt upgrade -y && apt-mark unhold libcap2 && clean-install ca-certificates uuid-dev util-linux mount udev wget e2fsprogs nfs-common netbase

ARG ARCH=amd64
RUN if [ "$ARCH" = "amd64" ] ; then \
  clean-install libcurl4-gnutls-dev && \
  wget -O /blobfuse-proxy/packages-microsoft-prod.deb https://packages.microsoft.com/config/ubuntu/18.04/packages-microsoft-prod.deb && \
  dpkg -i /blobfuse-proxy/packages-microsoft-prod.deb && apt update && apt install blobfuse blobfuse2 fuse -y && apt remove wget -y; fi
LABEL maintainers="andyzhangx"
LABEL description="Azure Blob Storage CSI driver"

ENTRYPOINT ["/blobplugin"]
