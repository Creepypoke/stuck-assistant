.PHONY: build run run-service install test fmt vet tidy clean

CLI := walkthrough-assistance
SERVICE := walkthrough-assistance-service
BIN_DIR := bin
INSTALL_DIR ?= $(HOME)/.local/share/walkthrough-assistance

build:
	mkdir -p $(BIN_DIR)/llama-cpp
	go build -o $(BIN_DIR)/$(CLI) ./cmd/$(CLI)
	go build -o $(BIN_DIR)/$(SERVICE) ./cmd/$(SERVICE)
	test -f $(BIN_DIR)/.env || cp .env.example $(BIN_DIR)/.env
	cp llama-cpp/models.ini $(BIN_DIR)/llama-cpp/models.ini

run:
	go run ./cmd/$(CLI)

run-service:
	go run ./cmd/$(SERVICE)

install: build
	install -d $(INSTALL_DIR)/bin $(INSTALL_DIR)/llama-cpp
	install -m 0755 $(BIN_DIR)/$(CLI) $(INSTALL_DIR)/bin/$(CLI)
	install -m 0755 $(BIN_DIR)/$(SERVICE) $(INSTALL_DIR)/bin/$(SERVICE)
	install -m 0644 llama-cpp/models.ini $(INSTALL_DIR)/llama-cpp/models.ini
	$(INSTALL_DIR)/bin/$(CLI) service install \
		--service-binary $(INSTALL_DIR)/bin/$(SERVICE)

test:
	go test $$(rg --files -g '*.go' -g '!tmp/**' -g '!vendor/**' | xargs -n1 dirname | sort -u | sed 's|^|./|')

fmt:
	gofmt -w $$(rg --files -g '*.go' -g '!tmp/**' -g '!vendor/**')

vet:
	go vet $$(rg --files -g '*.go' -g '!tmp/**' -g '!vendor/**' | xargs -n1 dirname | sort -u | sed 's|^|./|')

tidy:
	go mod tidy

clean:
	rm -rf bin
