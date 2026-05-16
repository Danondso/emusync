.PHONY: build test test-e2e clean docker install bootstrap-server

BINARY := emusync
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
# Docker Compose v2 CLI; set DOCKER_COMPOSE=docker-compose if you use the standalone binary
DOCKER_COMPOSE ?= docker compose
# Compose interpolates ${EMUSYNC_*}; exported EMUSYNC_* in your shell override project .env — clear so .env wins.
DOCKER_ENV ?= env -u EMUSYNC_AUTH_TOKEN -u EMUSYNC_ADMIN_TOKEN -u EMUSYNC_MAX_BACKUPS -u EMUSYNC_ADVERTISE_MDNS

build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./... -v

test-e2e:
	go test -tags e2e -v -timeout 30m ./tests/e2e/...

clean:
	rm -f $(BINARY)

docker:
	$(DOCKER_ENV) $(DOCKER_COMPOSE) build

docker-up:
	$(DOCKER_ENV) $(DOCKER_COMPOSE) up -d

docker-down:
	$(DOCKER_ENV) $(DOCKER_COMPOSE) down

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
