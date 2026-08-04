.PHONY: build build-pi deploy-pi test vet check run test-api

build:
	mkdir -p bin
	go build -trimpath -o bin/raspi-media-player ./cmd/raspi-media-player

build-pi:
	./scripts/build-pi.sh

deploy-pi:
	./scripts/deploy-pi.sh

test:
	go test ./...

vet:
	go vet ./...

check: vet test

run:
	go run ./cmd/raspi-media-player

test-api:
	./scripts/test-all.sh
