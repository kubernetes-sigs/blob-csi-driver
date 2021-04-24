# Copyright 2017 The Kubernetes Authors.
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

PKG = sigs.k8s.io/blob-csi-driver
GIT_COMMIT ?= $(shell git rev-parse HEAD)
REGISTRY ?= andyzhangx
REGISTRY_NAME ?= $(shell echo $(REGISTRY) | sed "s/.azurecr.io//g")
IMAGE_NAME ?= blob-csi
IMAGE_VERSION ?= v1.1.0
CLOUD ?= AzurePublicCloud
# Use a custom version for E2E tests if we are in Prow
ifdef CI
ifndef PUBLISH
override IMAGE_VERSION := e2e-$(GIT_COMMIT)
endif
endif
IMAGE_TAG ?= $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_VERSION)
IMAGE_TAG_LATEST = $(REGISTRY)/$(IMAGE_NAME):latest
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS ?= "-X ${PKG}/pkg/blob.driverVersion=${IMAGE_VERSION} -X ${PKG}/pkg/blob.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/blob.buildDate=${BUILD_DATE} -s -w -extldflags '-static'"
E2E_HELM_OPTIONS ?= --set image.blob.pullPolicy=Always --set image.blob.repository=$(REGISTRY)/$(IMAGE_NAME) --set image.blob.tag=$(IMAGE_VERSION)
ifdef ENABLE_BLOBFUSE_PROXY
override E2E_HELM_OPTIONS := $(E2E_HELM_OPTIONS) --set controller.logLevel=6 --set node.logLevel=6 --set node.enableBlobfuseProxy=true
endif
GINKGO_FLAGS = -ginkgo.v
GO111MODULE = on
GOPATH ?= $(shell go env GOPATH)
GOBIN ?= $(GOPATH)/bin
DOCKER_CLI_EXPERIMENTAL = enabled
export GOPATH GOBIN GO111MODULE DOCKER_CLI_EXPERIMENTAL

all: blob

.PHONY: verify
verify: unit-test
	hack/verify-all.sh

.PHONY: unit-test
unit-test:
	go test -covermode=count -coverprofile=profile.cov ./pkg/... ./test/utils/credentials

.PHONY: sanity-test
sanity-test: blob
	go test -v -timeout=30m ./test/sanity

.PHONY: integration-test
integration-test: blob
	go test -v -timeout=30m ./test/integration

.PHONY: e2e-test
e2e-test:
	if [ ! -z "$(EXTERNAL_E2E_TEST)" ]; then \
		bash ./test/external-e2e/run.sh;\
	else \
		go test -v -timeout=0 ./test/e2e ${GINKGO_FLAGS};\
	fi

.PHONY: e2e-bootstrap
e2e-bootstrap: install-helm install-blobfuse-proxy
	# Only build and push the image if it does not exist in the registry
	docker pull $(IMAGE_TAG) || make blob-container push
	if [[ -z "$(ENABLE_BLOBFUSE_PROXY)" ]]; then \
		make install-blobfuse-proxy;\
	fi
	helm install blob-csi-driver ./charts/latest/blob-csi-driver --namespace kube-system --wait --timeout=15m -v=5 --debug \
		--set controller.runOnMaster=true \
		--set controller.replicas=1 \
		--set cloud=$(CLOUD) \
		$(E2E_HELM_OPTIONS)

.PHONY: install-helm
install-helm:
	curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash

.PHONY: e2e-teardown
e2e-teardown:
	helm delete blob-csi-driver --namespace kube-system

.PHONY: blob
blob:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags ${LDFLAGS} -mod vendor -o _output/blobplugin ./pkg/blobplugin

.PHONY: blob-windows
blob-windows:
	CGO_ENABLED=0 GOOS=windows go build -a -ldflags ${LDFLAGS} -mod vendor -o _output/blobplugin.exe ./pkg/blobplugin

.PHONT: blob-darwin
blob-darwin:
	CGO_ENABLED=0 GOOS=darwin go build -a -ldflags ${LDFLAGS} -mod vendor -o _output/blobplugin ./pkg/blobplugin

.PHONY: container
container: blob
	docker build -t $(IMAGE_TAG) -f ./pkg/blobplugin/dev.Dockerfile .

.PHONY: blob-container
blob-container:
	docker buildx rm container-builder || true
	docker buildx create --use --name=container-builder
ifdef CI
ifeq ($(CLOUD), AzureStackCloud)
	docker run --privileged --name buildx_buildkit_container-builder0 -d --mount type=bind,src=/etc/ssl/certs,dst=/etc/ssl/certs moby/buildkit:latest || true
endif
	docker buildx build --no-cache --build-arg LDFLAGS=${LDFLAGS} -t $(IMAGE_TAG) -f ./pkg/blobplugin/Dockerfile --platform="linux/amd64" --push .

	docker manifest create $(IMAGE_TAG) $(IMAGE_TAG)
	docker manifest inspect $(IMAGE_TAG)
ifdef PUBLISH
	docker manifest create $(IMAGE_TAG_LATEST) $(IMAGE_TAG)
	docker manifest inspect $(IMAGE_TAG_LATEST)
endif
endif

.PHONY: push
push:
ifdef CI
	docker manifest push --purge $(IMAGE_TAG)
else
	docker push $(IMAGE_TAG)
endif

.PHONY: push-latest
push-latest:
ifdef CI
	docker manifest push --purge $(IMAGE_TAG_LATEST)
else
	docker push $(IMAGE_TAG_LATEST)
endif

.PHONY: build-push
build-push: blob-container
	docker tag $(IMAGE_TAG) $(IMAGE_TAG_LATEST)
	docker push $(IMAGE_TAG_LATEST)

.PHONY: clean
clean:
	go clean -r -x
	-rm -rf _output

.PHONY: create-metrics-svc
create-metrics-svc:
	kubectl create -f deploy/example/metrics/csi-blob-controller-svc.yaml

.PHONY: delete-metrics-svc
delete-metrics-svc:
	kubectl delete -f deploy/example/metrics/csi-blob-controller-svc.yaml --ignore-not-found

# compile .proto file to go output
.PHONY: gen-proto
gen-proto:
	protoc --proto_path=pkg/blobfuse-proxy/proto --go-grpc_out=pkg/blobfuse-proxy/pb --go_out=pkg/blobfuse-proxy/pb pkg/blobfuse-proxy/proto/*.proto

# clean the files generated from .proto file
.PHONY: clean-proto
clean-proto:
	rm pkg/blobfuse-proxy/pb/*.go

.PHONY: blobfuse-proxy
blobfuse-proxy:
	CGO_ENABLED=0 GOOS=linux go build -mod vendor -ldflags="-s -w" -o _output/blobfuse-proxy ./pkg/blobfuse-proxy

.PHONY: blobfuse-proxy-container
blobfuse-proxy-container:
	sudo docker build -t blobfuse-proxy -f pkg/blobfuse-proxy/Dockerfile .

.PHONY: install-blobfuse-proxy
install-blobfuse-proxy:
	kubectl apply -f ./deploy/blobfuse-proxy/blobfuse-proxy.yaml

.PHONY: uninstall-blobfuse-proxy
uninstall-blobfuse-proxy:
	kubectl delete -f ./deploy/blobfuse-proxy/blobfuse-proxy.yaml --ignore-not-found

.PHONY: setup-external-e2e
setup-external-e2e:
	curl -sL https://storage.googleapis.com/kubernetes-release/release/v1.19.0/kubernetes-test-linux-amd64.tar.gz --output e2e-tests.tar.gz
	tar -xvf e2e-tests.tar.gz
	rm e2e-tests.tar.gz
	mkdir /tmp/csi-blobfuse
	cp ./kubernetes/test/bin/e2e.test /tmp/csi-blobfuse/e2e.test
	rm -r kubernetes
	cp ./deploy/example/storageclass-blobfuse.yaml /tmp/csi-blobfuse/storageclass.yaml
	cp ./test/e2e-external/testdriver.yaml /tmp/csi-blobfuse/testdriver.yaml
	./deploy/install-driver.sh

.PHONY: run-external-e2e
run-external-e2e: setup-external-e2e install-blobfuse-proxy
	bash ./test/e2e-external/run.sh
