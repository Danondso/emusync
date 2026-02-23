.PHONY: build test clean docker install

BINARY := emusync
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./... -v

clean:
	rm -f $(BINARY)

docker:
	docker-compose build

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

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
