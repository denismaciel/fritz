APP := fritz
BIN := bin/$(APP)
PROMPT ?= hi
PREFIX ?= $(HOME)/.local
INSTALL_BIN := $(PREFIX)/bin/$(APP)

.PHONY: help build install run chat doctor fmt test clean

help:
	@printf "targets:\n"
	@printf "  make build   build binary\n"
	@printf "  make install install binary to $(PREFIX)/bin\n"
	@printf "  make run     run one-shot prompt\n"
	@printf "  make chat    start chat loop\n"
	@printf "  make doctor  check env\n"
	@printf "  make fmt     format go code\n"
	@printf "  make test    run tests\n"
	@printf "  make clean   remove build output\n"

build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/$(APP)

install: build
	@mkdir -p $(PREFIX)/bin
	install -m 755 $(BIN) $(INSTALL_BIN)

run:
	go run ./cmd/$(APP) run "$(PROMPT)"

chat:
	go run ./cmd/$(APP) chat

doctor:
	go run ./cmd/$(APP) doctor

fmt:
	gofmt -w $$(find . -type f -name '*.go')

test:
	go test ./...

clean:
	rm -rf bin
