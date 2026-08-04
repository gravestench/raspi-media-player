.PHONY: build build-pi build-release deploy-pi test vet check run test-api check-pi-dependencies

build:
	mkdir -p bin
	go build -trimpath -o bin/raspi-media-player ./cmd/raspi-media-player

build-pi:
	./scripts/build-pi.sh

build-release:
	@test -n "$(VERSION)" || { echo "Usage: make build-release VERSION=v0.2.0" >&2; exit 2; }
	./scripts/build-release.sh "$(VERSION)"

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
