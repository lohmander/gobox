BINARY_NAME=gobox
BUILD_DIR=build

all: build

test:
	@echo "Running tests..."
	go test ./... -v

build: test
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/$(BINARY_NAME)/main.go
	@echo "Build complete. Binary in $(BUILD_DIR)/$(BINARY_NAME)"

install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	mkdir -p /usr/local/bin
	sudo install $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "$(BINARY_NAME) installed to /usr/local/bin"

clean:
	@echo "Cleaning build directory..."
	rm -rf $(BUILD_DIR)

.PHONY: all build install clean
