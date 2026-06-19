.PHONY: build test fmt

build:
	go build -tags lsf -o lsf-exporter ./cmd/lsf-exporter

test:
	go test ./...

fmt:
	gofmt -w cmd internal
