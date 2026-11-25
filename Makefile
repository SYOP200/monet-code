SHELL := /bin/bash
BINARY := bin/monet-tui
PYTHON ?= python3
VENV := .venv
PIP := $(VENV)/bin/pip
MONET := $(VENV)/bin/monet

.PHONY: all setup build go-build cli sync-catalog run tui test fmt vet clean doctor help

all: build

help:
	@echo "Monet Code"
	@echo "  make setup       create .venv, install the CLI, download Go modules"
	@echo "  make build       build the Go TUI and install the Python CLI"
	@echo "  make run         build, then launch the TUI"
	@echo "  make test        run Go + Python tests"
	@echo "  make fmt         format Go and byte-compile Python"
	@echo "  make clean       remove build artefacts and the virtualenv"

setup: sync-catalog
	$(PYTHON) -m venv $(VENV)
	$(PIP) install --upgrade pip
	$(PIP) install -e .
	go mod download

build: sync-catalog go-build cli

go-build:
	@mkdir -p bin
	go build -o $(BINARY) ./cmd/monet-tui

cli:
	@if [ ! -x "$(MONET)" ]; then \
		echo "Python CLI not installed. Run 'make setup' first."; \
		exit 1; \
	fi
	$(PIP) install -e . >/dev/null

sync-catalog:
	@mkdir -p monet_cli
	@cp internal/catalog/models.json monet_cli/models.json

run: build
	$(MONET) tui

tui: run

test: sync-catalog
	go test ./...
	$(VENV)/bin/python -m unittest discover -s tests -v

fmt:
	gofmt -w cmd internal
	$(PYTHON) -m compileall -q monet_cli

vet:
	go vet ./...

doctor: build
	$(MONET) doctor

clean:
	rm -rf bin dist build *.egg-info monet_cli/__pycache__ tests/__pycache__
