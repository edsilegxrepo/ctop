NAME=ctop
BIN=bin
VERSION=$(shell cat version.txt)
BUILD=$(shell git rev-parse --short HEAD)
GO_LD_FLAGS="-w -X main.version=$(VERSION) -X main.build=$(BUILD)"
GO_OPTS=-trimpath -buildmode=pie

clean:
	rm -rf $(BIN)/ _build/ release/

build:
	go mod download
	mkdir -p $(BIN)
	CGO_ENABLED=0 go build -tags release -ldflags $(GO_LD_FLAGS) $(GO_OPTS) -o $(BIN)/$(NAME)
	$(BIN)/$(NAME) --version

build-all:
	mkdir -p $(BIN)
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -tags release -ldflags $(GO_LD_FLAGS) $(GO_OPTS) -o $(BIN)/ctop-$(VERSION)-linux-amd64
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags release -ldflags $(GO_LD_FLAGS) $(GO_OPTS) -o $(BIN)/ctop-$(VERSION)-windows-amd64.exe
	cd $(BIN) && sha256sum ctop-* > sha256sums.txt

run-dev:
	rm -f ctop.sock $(BIN)/ctop
	mkdir -p $(BIN)
	go build -ldflags $(GO_LD_FLAGS) -o $(BIN)/ctop
	CTOP_DEBUG=1 ./$(BIN)/ctop

image:
	docker build -t ctop -f Dockerfile .

release:
	mkdir -p release
	cp $(BIN)/* release/
	cd release && sha256sum --quiet --check sha256sums.txt && \
	gh release create v$(VERSION) -d -t v$(VERSION) *

.PHONY: clean build build-all run-dev image release
