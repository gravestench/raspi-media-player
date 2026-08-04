.PHONY: build build-pi deploy-pi test vet check run test-api check-pi-dependencies

build:
	mkdir -p bin
	go build -trimpath -o bin/raspi-media-player ./cmd/raspi-media-player

build-pi:
	./scripts/build-pi.sh

deploy-pi:
	./scripts/deploy-pi.sh

check-pi-dependencies:
	./scripts/install-dependencies.sh --check

test:
	go test ./...

vet:
	go vet ./...

check: vet test

run:
	go run ./cmd/raspi-media-player

test-api:
	./scripts/test-all.sh
