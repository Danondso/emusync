.PHONY: build test test-e2e clean docker install bootstrap-server

BINARY := emusync
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
# Docker Compose v2 CLI; set DOCKER_COMPOSE=docker-compose if you use the standalone binary
DOCKER_COMPOSE ?= docker compose
# Compose interpolates ${EMUSYNC_*}; exported EMUSYNC_* in your shell override project .env — bash unset keeps .env authoritative.
build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./... -v

test-e2e:
	go test -tags e2e -v -timeout 30m ./tests/e2e/...

clean:
	rm -f $(BINARY)

docker:
	bash -c 'unset EMUSYNC_AUTH_TOKEN EMUSYNC_ADMIN_TOKEN EMUSYNC_MAX_BACKUPS EMUSYNC_ADVERTISE_MDNS; exec $(DOCKER_COMPOSE) build'

docker-up:
	bash -c 'unset EMUSYNC_AUTH_TOKEN EMUSYNC_ADMIN_TOKEN EMUSYNC_MAX_BACKUPS EMUSYNC_ADVERTISE_MDNS; exec $(DOCKER_COMPOSE) up -d'

docker-down:
	bash -c 'unset EMUSYNC_AUTH_TOKEN EMUSYNC_ADMIN_TOKEN EMUSYNC_MAX_BACKUPS EMUSYNC_ADVERTISE_MDNS; exec $(DOCKER_COMPOSE) down'

bootstrap-server:
	./scripts/bootstrap-server.sh

install: build
	mkdir -p ~/.local/bin
	cp $(BINARY) ~/.local/bin/$(BINARY)
	@echo "Installed to ~/.local/bin/$(BINARY)"

install-service:
	mkdir -p ~/.config/systemd/user
	cp deploy/systemd/emusync-watch.service ~/.config/systemd/user/
	systemctl --user daemon-reload
	@echo "Service installed. Enable with: systemctl --user enable --now emusync-watch"

init: build
	./$(BINARY) init
