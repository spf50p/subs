BINARY := subs
BIN_DIR := .bin

.PHONY: build build-linux build-all clean test vet

# Build for the local system.
build:
	go build -o $(BIN_DIR)/$(BINARY) .

# Build for linux/amd64.
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BIN_DIR)/$(BINARY)-linux-amd64 .

# Build both targets.
build-all: build build-linux

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf $(BIN_DIR)
