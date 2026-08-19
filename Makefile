NAME=ctop
VERSION=$(shell cat VERSION)
BUILD=$(shell git rev-parse --short HEAD)
GO_LD_FLAGS="-w -X main.version=$(VERSION) -X main.build=$(BUILD)"
GO_OPTS=-trimpath -buildmode=pie

clean:
	rm -rf _build/ release/

build:
	go mod download
	CGO_ENABLED=0 go build -tags release -ldflags $(GO_LD_FLAGS) $(GO_OPTS) -o ctop

build-all:
	mkdir -p _build
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -tags release -ldflags $(GO_LD_FLAGS) $(GO_OPTS) -o _build/ctop-$(VERSION)-linux-amd64
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags release -ldflags $(GO_LD_FLAGS) $(GO_OPTS) -o _build/ctop-$(VERSION)-windows-amd64.exe
	cd _build && sha256sum * > sha256sums.txt

run-dev:
	rm -f ctop.sock ctop
	go build -ldflags $(GO_LD_FLAGS) -o ctop
	CTOP_DEBUG=1 ./ctop

image:
	docker build -t ctop -f Dockerfile .

release:
	mkdir -p release
	cp _build/* release/
	cd release && sha256sum --quiet --check sha256sums.txt && \
	gh release create v$(VERSION) -d -t v$(VERSION) *

.PHONY: clean build build-all run-dev image release
