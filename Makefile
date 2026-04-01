BINARY    := podvirt
BUILD_DIR := ./bin

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   := -ldflags "-X main.version=$(VERSION)"

# Required build tags to exclude C-library-dependent storage drivers
# when using Podman bindings in a client-only binary.
BUILD_TAGS := exclude_graphdriver_btrfs exclude_graphdriver_devicemapper containers_image_openpgp

.PHONY: all build install clean test lint deps-update

all: build

build:
	mkdir -p $(BUILD_DIR)
	go build -tags "$(BUILD_TAGS)" $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) .

install:
	go install -tags "$(BUILD_TAGS)" $(LDFLAGS) .

clean:
	rm -rf $(BUILD_DIR)

test:
	go test -tags "$(BUILD_TAGS)" ./...

lint:
	go vet -tags "$(BUILD_TAGS)" ./...


deps-update:
	go get -u -t ./...
	go mod tidy
