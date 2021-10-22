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

FROM k8s.gcr.io/build-image/debian-base:bullseye-v1.0.0
COPY ./_output/blobplugin /blobplugin
RUN apt update && apt-mark unhold libcap2
RUN clean-install ca-certificates libfuse-dev libcurl4-gnutls-dev libgnutls28-dev uuid-dev libgcrypt20-dev util-linux mount udev wget e2fsprogs nfs-common
# install updated packages to fix CVE issues
RUN clean-install libssl1.1 libgssapi-krb5-2 libk5crypto3 libkrb5-3 libkrb5support0
RUN mkdir /blobfuse-proxy/
COPY ./_output/blobfuse-proxy.deb /blobfuse-proxy/
# for compatibility, remove this after v1.6.0 release (todo)
COPY deploy/blobfuse-proxy/v0.1.0/blobfuse-proxy-v0.1.0.deb /blobfuse-proxy/
RUN wget -O /blobfuse-proxy/packages-microsoft-prod.deb https://packages.microsoft.com/config/ubuntu/18.04/packages-microsoft-prod.deb
RUN dpkg -i /blobfuse-proxy/packages-microsoft-prod.deb && apt update && apt install blobfuse fuse -y
# this is a workaround to install nfs-kernel-server and don't quit with error
RUN apt install nfs-kernel-server -y || true
RUN apt remove wget -y
LABEL maintainers="andyzhangx"
LABEL description="Azure Blob Storage CSI driver"

ENTRYPOINT ["/blobplugin"]

