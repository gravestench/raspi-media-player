.PHONY: build test vet check run test-api

build:
	mkdir -p bin
	go build -trimpath -o bin/raspi-media-player ./cmd/raspi-media-player

test:
	go test ./...

vet:
	go vet ./...

check: vet test

run:
	go run ./cmd/raspi-media-player

test-api:
	./scripts/test-all.sh
