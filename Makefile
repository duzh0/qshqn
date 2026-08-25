MAIN    := cmd/qshqn/main.go
OUT_DIR := out
BIN     := $(OUT_DIR)/qshqn
TAGS    :=
LDFLAGS :=

.PHONY: all build release vps-build vps-release gen-build start test tidy release-deploy deploy dump

all: build

build:
	go build $(TAGS) $(LDFLAGS) -o $(BIN) $(MAIN)

release: LDFLAGS += -ldflags "-s -w"
release: build

vps-build vps-release: BIN := $(BIN)-vps
vps-build vps-release: TAGS += -tags embed

vps-build: build
vps-release: release

gen-build:
	go generate ./...
	$(MAKE) build

start: build
	./$(OUT_DIR)/qshqn

tidy:
	go mod tidy

test:
	go test -v ./...

release-deploy: vps-release
	./sh/deploy.sh

deploy:
	./sh/deploy.sh

dump: 
	./sh/dump.sh