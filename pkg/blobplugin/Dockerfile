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

FROM golang:1.13.10-alpine3.10 as builder
WORKDIR /go/src/sigs.k8s.io/blob-csi-driver
ADD . .
ARG TARGETARCH
ARG TARGETOS
ARG LDFLAGS
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -a -ldflags "${LDFLAGS:--X sigs.k8s.io/blob-csi-driver/pkg/blob.driverVersion=latest}" -o _output/blobplugin ./pkg/blobplugin

FROM us.gcr.io/k8s-artifacts-prod/build-image/debian-base-amd64:v2.1.0
COPY --from=builder /go/src/sigs.k8s.io/blob-csi-driver/_output/blobplugin /blobplugin
RUN apt-get update && clean-install ca-certificates pkg-config libfuse-dev cmake libcurl4-gnutls-dev libgnutls28-dev uuid-dev libgcrypt20-dev wget
RUN wget -O /tmp/packages-microsoft-prod.deb https://packages.microsoft.com/config/ubuntu/16.04/packages-microsoft-prod.deb && dpkg -i /tmp/packages-microsoft-prod.deb && apt-get update && apt install blobfuse fuse -y && rm -f /tmp/packages-microsoft-prod.deb
RUN apt remove wget -y
RUN apt update && apt install mount nfs-common -y
LABEL maintainers="andyzhangx"
LABEL description="Azure Blob Storage CSI driver"

ENTRYPOINT ["/blobplugin"]
