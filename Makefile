SCRIPTS_DIR ?= $(HOME)/Development/github.com/rios0rios0/pipelines
-include $(SCRIPTS_DIR)/makefiles/common.mk
-include $(SCRIPTS_DIR)/makefiles/golang.mk

VERSION ?= $(shell { git describe --tags --abbrev=0 2>/dev/null || echo "dev"; } | sed 's/^v//')
LDFLAGS := -X main.version=$(VERSION)

.PHONY: debug build install run

build:
	rm -rf bin
	go build -ldflags "$(LDFLAGS) -s -w" -o bin/ccswitch ./cmd/ccswitch

debug:
	rm -rf bin
	go build -gcflags "-N -l" -ldflags "$(LDFLAGS)" -o bin/ccswitch ./cmd/ccswitch

run:
	go run ./cmd/ccswitch

install:
	make build
	mkdir -p ~/.local/bin
	cp -v bin/ccswitch ~/.local/bin/ccswitch
