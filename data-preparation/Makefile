.PHONY: build test vet fmt
build:
	mkdir -p bin
	go build -o bin/data-preparation ./cmd/data-preparation
test:
	go test ./...
vet:
	go vet ./...
fmt:
	gofmt -w $$(rg --files -g '*.go' -g '!bin/**')
