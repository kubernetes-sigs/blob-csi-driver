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

ARG ARCH

FROM k8s.gcr.io/build-image/debian-base:buster-v1.6.0
COPY ./_output/blobplugin /blobplugin
RUN apt-get update && clean-install ca-certificates pkg-config libfuse-dev cmake libcurl4-gnutls-dev libgnutls28-dev uuid-dev libgcrypt20-dev wget
RUN wget -O /tmp/packages-microsoft-prod.deb https://packages.microsoft.com/config/ubuntu/18.04/packages-microsoft-prod.deb && dpkg -i /tmp/packages-microsoft-prod.deb && apt-get update && apt install blobfuse fuse -y && rm -f /tmp/packages-microsoft-prod.deb
RUN apt remove wget -y
# this is a workaround to install nfs-common & nfs-kernel-server and don't quit with error
RUN apt-get update && apt install nfs-common nfs-kernel-server -y || true
LABEL maintainers="andyzhangx"
LABEL description="Azure Blob Storage CSI driver"

ENTRYPOINT ["/blobplugin"]
