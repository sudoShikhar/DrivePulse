# DrivePulse Makefile
APP_NAME := DrivePulse
SRC_DIR := ./src
BUILDS_DIR := builds
VERSION ?= 1.0.0

ifeq ($(OS),Windows_NT)
	BUILD_DATE := $(shell powershell -NoProfile -Command "Get-Date -Format 'yyyy-MM-dd'")
else
	BUILD_DATE := $(shell date +'%Y-%m-%d')
endif

# Go linker flags
LDFLAGS_WINDOWS := -H=windowsgui -s -w -X 'main.Version=$(VERSION)' -X 'main.BuildDate=$(BUILD_DATE)'
LDFLAGS_LINUX := -s -w -X 'main.Version=$(VERSION)' -X 'main.BuildDate=$(BUILD_DATE)'

.PHONY: help clean setup lint format test run build

## help: Show available targets
help:
	@echo DrivePulse Build Targets:
	@echo   make clean  - Remove $(BUILDS_DIR)/ directory
	@echo   make setup  - Download and tidy Go dependencies
	@echo   make lint   - Run static analysis without modifying files
	@echo   make format - Format code, organize imports, and run deep static analysis
	@echo   make test   - Run unit tests with code coverage
	@echo   make run    - Run locally directly from source
	@echo   make build  - Compile Windows and Linux binaries and patch PE resources

## clean: Remove build artifacts
clean:
ifeq ($(OS),Windows_NT)
	@powershell -NoProfile -Command "if (Test-Path '$(BUILDS_DIR)') { Get-ChildItem -Path '$(BUILDS_DIR)' -Recurse | Remove-Item -Force -Recurse -ErrorAction SilentlyContinue; Remove-Item -Path '$(BUILDS_DIR)' -Force -ErrorAction SilentlyContinue; exit 0 }"
else
	@rm -rf $(BUILDS_DIR)
endif

## setup: Download and tidy Go dependencies
setup:
	go mod download
	go mod tidy

## lint: Run static analysis and vet checks
lint:
	go vet ./...
	go tool staticcheck ./...

## format: Format code, organize imports, and run deep static analysis
format:
	gofmt -s -w .
	go tool goimports -w .
	$(MAKE) lint

## test: Run unit tests with code coverage
test:
	go test -v -cover ./...

## run: Run locally directly from source
run:
	go run $(SRC_DIR) -in-place

## build: Cross-compile for Windows and Linux and patch Windows PE resources
build:
ifeq ($(OS),Windows_NT)
	@if not exist $(BUILDS_DIR) mkdir $(BUILDS_DIR)
	powershell -NoProfile -Command "$$env:CGO_ENABLED='0'; $$env:GOOS='windows'; $$env:GOARCH='amd64'; go build -ldflags=\"$(LDFLAGS_WINDOWS)\" -o $(BUILDS_DIR)/$(APP_NAME)-windows-x64.exe $(SRC_DIR)"
	go tool go-winres patch --in src/winres/winres.json --no-backup $(BUILDS_DIR)/$(APP_NAME)-windows-x64.exe
	powershell -NoProfile -Command "$$env:CGO_ENABLED='0'; $$env:GOOS='linux'; $$env:GOARCH='amd64'; go build -ldflags=\"$(LDFLAGS_LINUX)\" -o $(BUILDS_DIR)/$(APP_NAME)-linux-x64 $(SRC_DIR)"
else
	mkdir -p $(BUILDS_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS_WINDOWS)" -o $(BUILDS_DIR)/$(APP_NAME)-windows-x64.exe $(SRC_DIR)
	go tool go-winres patch --in src/winres/winres.json --no-backup $(BUILDS_DIR)/$(APP_NAME)-windows-x64.exe
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS_LINUX)" -o $(BUILDS_DIR)/$(APP_NAME)-linux-x64 $(SRC_DIR)
endif
