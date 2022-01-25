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

FROM k8s.gcr.io/build-image/debian-base:bullseye-v1.1.0

ARG ARCH=amd64
COPY ./_output/${ARCH}/blobplugin /blobplugin

RUN apt update && apt-mark unhold libcap2
RUN clean-install ca-certificates uuid-dev util-linux mount udev wget e2fsprogs nfs-common netbase
# install updated packages to fix CVE issues
RUN clean-install libgmp10 bsdutils
RUN mkdir /blobfuse-proxy/
COPY ./_output/blobfuse-proxy.deb /blobfuse-proxy/
# for compatibility, remove this after v1.6.0 release (todo)
COPY deploy/blobfuse-proxy/v0.1.0/blobfuse-proxy-v0.1.0.deb /blobfuse-proxy/
ARG ARCH=amd64
RUN if [ "$ARCH" = "amd64" ] ; then \
  clean-install libcurl4-gnutls-dev && \
  wget -O /blobfuse-proxy/packages-microsoft-prod.deb https://packages.microsoft.com/config/ubuntu/18.04/packages-microsoft-prod.deb && \
  dpkg -i /blobfuse-proxy/packages-microsoft-prod.deb && apt update && apt install blobfuse fuse -y && apt remove wget -y; fi
LABEL maintainers="andyzhangx"
LABEL description="Azure Blob Storage CSI driver"

ENTRYPOINT ["/blobplugin"]

