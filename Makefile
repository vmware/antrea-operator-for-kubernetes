SHELL := /bin/bash
LDFLAGS :=

# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Default to Openshift for the platform, which means for example that the Antrea
# image will default to antrea-ubi in generate manifests. To use the operator
# with a "vanilla" (non-Openshift) K8s cluster, you will need to explicitly set
# ANTREA_PLATFORM=kubernetes when generating resources.
ifndef ANTREA_PLATFORM
	ANTREA_PLATFORM=openshift
endif

ifndef IS_CERTIFICATION
	IS_CERTIFICATION=false
endif

.PHONY: all
all: generate golangci manager

include versioning.mk
LDFLAGS += $(VERSION_LDFLAGS)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Options for "packagemanifests".
ifneq ($(origin FROM_VERSION), undefined)
PKG_FROM_VERSION := --from-version=$(FROM_VERSION)
endif
ifneq ($(origin CHANNEL), undefined)
PKG_CHANNELS := --channel=$(CHANNEL)
endif
ifeq ($(IS_CHANNEL_DEFAULT), 1)
PKG_IS_DEFAULT_CHANNEL := --default-channel
endif
PKG_MAN_OPTS ?= $(FROM_VERSION) $(PKG_CHANNELS) $(PKG_IS_DEFAULT_CHANNEL)

GOLANGCI_LINT_VERSION := v1.60.3
GOLANGCI_LINT_BINDIR  := $(CURDIR)/.golangci-bin
GOLANGCI_LINT_BIN     := $(GOLANGCI_LINT_BINDIR)/$(GOLANGCI_LINT_VERSION)/golangci-lint

$(GOLANGCI_LINT_BIN):
	@echo "===> Installing Golangci-lint <==="
	@rm -rf $(GOLANGCI_LINT_BINDIR)/* # remove old versions
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOLANGCI_LINT_BINDIR)/$(GOLANGCI_LINT_VERSION) $(GOLANGCI_LINT_VERSION)

.PHONY: golangci
golangci: $(GOLANGCI_LINT_BIN)
	@echo "===> Running golangci <==="
	@GOOS=linux $(GOLANGCI_LINT_BIN) run -c $(CURDIR)/.golangci.yml

.PHONY: golangci-fix
golangci-fix: $(GOLANGCI_LINT_BIN)
	@echo "===> Running golangci-fix <==="
	@GOOS=linux $(GOLANGCI_LINT_BIN) run -c $(CURDIR)/.golangci.yml --fix

# Run tests
ENVTEST_ASSETS_DIR = $(shell pwd)/testbin
test: generate golangci manifests
	mkdir -p $(ENVTEST_ASSETS_DIR)
	test -f $(ENVTEST_ASSETS_DIR)/setup-envtest.sh || curl -sSLo $(ENVTEST_ASSETS_DIR)/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.6.3/hack/setup-envtest.sh
	source $(ENVTEST_ASSETS_DIR)/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out

# Build manager binary
manager:
	@echo "===> Building antrea-operator binary <==="
	GOOS=linux go build -o bin/manager -ldflags '$(LDFLAGS)' main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate golangci manifests
	GOOS=linux go run -ldflags '$(LDFLAGS)' ./main.go

# Install CRDs into a cluster
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen kustomize antrea-resources
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=antrea-operator webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	$(KUSTOMIZE) build config/crd > config/crd/operator.antrea.vmware.com_antreainstalls.yaml

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build:
	docker build -f build/Dockerfile --platform linux/amd64 --label version="$(VERSION)" . -t ${IMG}
	docker tag ${IMG} antrea/antrea-operator

CONTROLLER_GEN_VERSION := v0.6.2
CONTROLLER_GEN_BINDIR  := $(CURDIR)/.controller-gen
CONTROLLER_GEN         := $(CONTROLLER_GEN_BINDIR)/$(CONTROLLER_GEN_VERSION)/controller-gen

$(CONTROLLER_GEN):
	@echo "===> Installing Controller-gen  <==="
	@rm -rf $(CONTROLLER_GEN_BINDIR)/* # remove old versions
	GOBIN=$(CONTROLLER_GEN_BINDIR)/$(CONTROLLER_GEN_VERSION) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)

KUSTOMIZE_VERSION := 5.3.0
KUSTOMIZE_BINDIR  := $(CURDIR)/.kustomize
KUSTOMIZE         := $(KUSTOMIZE_BINDIR)/$(KUSTOMIZE_VERSION)/kustomize

$(KUSTOMIZE):
	@echo "===> Installing Kustomize <==="
	@rm -rf $(KUSTOMIZE_BINDIR)/* # remove old versions
	@mkdir -p $(KUSTOMIZE_BINDIR)/$(KUSTOMIZE_VERSION)
	@curl -sSfL https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh | bash -s -- $(KUSTOMIZE_VERSION) $(KUSTOMIZE_BINDIR)/$(KUSTOMIZE_VERSION)

.PHONY: kustomize
kustomize: $(KUSTOMIZE)

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests kustomize
	operator-sdk generate kustomize manifests -q
	# OCP requires that an image will be identified by its digest hash
	if [ "$(IS_CERTIFICATION)" == "true" ]; then \
	        $(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --use-image-digests --overwrite $(BUNDLE_METADATA_OPTS) --version $(VERSION) ; \
	else \
		pushd config/manager && $(KUSTOMIZE) edit set image antrea/antrea-operator:v$(VERSION) && popd;\
	        $(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite $(BUNDLE_METADATA_OPTS) --version $(VERSION) ; \
	fi
	operator-sdk bundle validate ./bundle
	cp config/samples/operator_v1_antreainstall.yaml ./deploy/$(ANTREA_PLATFORM)/operator.antrea.vmware.com_v1_antreainstall_cr.yaml
	cp config/crd/operator.antrea.vmware.com_antreainstalls.yaml deploy/$(ANTREA_PLATFORM)/operator.antrea.vmware.com_antreainstalls_crd.yaml

.PHONY: ocpbundle
ocpbundle: ANTREA_PLATFORM=openshift
ocpbundle: bundle
	./hack/edit_bundle_metadata_ocp.sh

ocpcertification: IS_CERTIFICATION=true
ocpcertification: ocpbundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .
	docker tag ${BUNDLE_IMG} antrea/antrea-operator-bundle

antrea-resources:
	KUSTOMIZE=$(KUSTOMIZE) ./hack/generate-antrea-resources.sh --platform $(ANTREA_PLATFORM) --version $(VERSION)
	cp ./config/rbac/role.yaml ./deploy/$(ANTREA_PLATFORM)/role.yaml
	cp ./config/samples/operator_v1_antreainstall.yaml ./deploy/$(ANTREA_PLATFORM)/operator.antrea.vmware.com_v1_antreainstall_cr.yaml

# Generate package manifests.
packagemanifests: kustomize manifests
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image antrea-operator=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate packagemanifests -q --version $(VERSION) $(PKG_MAN_OPTS)

test-tidy:
	@echo
	@echo "===> Checking go.mod tidiness <==="
	./hack/tidy-check.sh

.PHONY: tidy
tidy:
	rm -f go.sum
	go mod tidy

.PHONY: bin
bin: manager

.PHONY: clean
clean:
	@rm -rf bin
	@rm -rf $(GOLANGCI_LINT_BINDIR)
	@rm -rf $(KUSTOMIZE_BINDIR)
	@rm -rf $(CONTROLLER_GEN_BINDIR)
