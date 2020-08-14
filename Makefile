# go options
GO                 ?= go
LDFLAGS            :=
GOFLAGS            :=
BINDIR             ?= $(CURDIR)/build/_output/bin
GO_FILES           := $(shell find . -type d -name '.cache' -prune -o -type f -name '*.go' -print)
GOPATH             ?= $$($(GO) env GOPATH)

.PHONY: all
all: build

LDFLAGS += $(VERSION_LDFLAGS)
OPERATOR_NAME = antrea-operator

.PHONY: build
build:
	@echo "===> Building antrea-operator Docker image <==="
	docker build -f build/Dockerfile . -t $(OPERATOR_NAME)

.PHONY: bin
bin:
	@echo "===> Building antrea-operator binary <==="
	GOOS=linux $(GO) build -o $(BINDIR)/$(OPERATOR_NAME) $(GOFLAGS) -ldflags '$(LDFLAGS)' ./cmd/manager

.PHONY: test-unit
test-unit:
	@echo "===> Running unit tests <==="
	GOOS=linux $(GO) test -race -cover github.com/ruicao93/antrea-operator/pkg...

.golangci-bin:
	@echo "===> Installing Golangci-lint <==="
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $@ v1.21.0

.PHONY: golangci
golangci: .golangci-bin
	@GOOS=linux CGO_ENABLED=1 .golangci-bin/golangci-lint run -c .golangci.yml

.PHONY: clean
clean:
	rm -f $(BINDIR)/$(OPERATOR_NAME)
