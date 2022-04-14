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

FROM registry.redhat.io/rhel:7.8-394
## TODO: make my own image which includes blobfuse based on rhel:7.8-394?

ARG ARCH=amd64
ARG binary=./_output/${ARCH}/blobplugin
COPY ${binary} /blobplugin

RUN mkdir /blobfuse-proxy/
COPY ./_output/blobfuse-proxy.rpm /blobfuse-proxy/
ARG ARCH=amd64
RUN if [ "$ARCH" = "amd64" ] ; then \
  # subscription-manager register && subscription-manager attach && \
  rpm -Uvh https://packages.microsoft.com/config/rhel/7/packages-microsoft-prod.rpm && \
  yum update && rpm -Uvh https://packages.microsoft.com/rhel/7.8/prod/blobfuse-1.4.3-RHEL-7.8-x86_64.rpm; fi
LABEL maintainers="andyzhangx"
LABEL description="Azure Blob Storage CSI driver"

ENTRYPOINT ["/blobplugin"]