BINARY      := gateway
PKG         := ./...
CMD         := ./cmd/gateway
BIN_DIR     := bin
GO          := go

.PHONY: all build run test test-race lint fmt vet tidy clean

all: build

build:
	$(GO) build -o $(BIN_DIR)/$(BINARY) $(CMD)

run:
	$(GO) run $(CMD)

test:
	$(GO) test $(PKG)

test-race:
	$(GO) test -race -count=1 $(PKG)

cover:
	$(GO) test -coverprofile=coverage.txt $(PKG)
	$(GO) tool cover -func=coverage.txt | tail -n 1

fmt:
	$(GO) fmt $(PKG)

vet:
	$(GO) vet $(PKG)

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR) coverage.txt coverage.html
