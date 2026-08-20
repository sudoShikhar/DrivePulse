# DrivePulse Makefile
APP_NAME := DrivePulse
SRC_DIR := ./src
BUILDS_DIR := builds
VERSION ?= 1.0.0

# Go linker flags
LDFLAGS_WINDOWS := -H=windowsgui -s -w -X 'main.Version=$(VERSION)'
LDFLAGS_LINUX := -s -w -X 'main.Version=$(VERSION)'

.PHONY: help clean test run build

## help: Show available targets
help:
	@echo DrivePulse Build Targets:
	@echo   make clean - Remove $(BUILDS_DIR)/ directory
	@echo   make test  - Run unit tests with code coverage
	@echo   make run   - Run locally directly from source
	@echo   make build - Compile Windows and Linux binaries and patch PE resources

## clean: Remove build artifacts
clean:
ifeq ($(OS),Windows_NT)
	@if exist $(BUILDS_DIR) rmdir /s /q $(BUILDS_DIR)
else
	@rm -rf $(BUILDS_DIR)
endif

## test: Run unit tests with code coverage
test:
	go test -v -cover ./...

## run: Run locally directly from source
run:
	go run $(SRC_DIR) -in-place

## build: Cross-compile for Windows and Linux and patch Windows PE resources
build:
ifeq ($(OS),Windows_NT)
	powershell -Command "$$env:CGO_ENABLED='0'; $$env:GOOS='windows'; $$env:GOARCH='amd64'; go build -ldflags=\"$(LDFLAGS_WINDOWS)\" -o $(BUILDS_DIR)/$(APP_NAME)-windows-x64.exe $(SRC_DIR); cd $(SRC_DIR); go run github.com/tc-hib/go-winres@latest patch --no-backup ../$(BUILDS_DIR)/$(APP_NAME)-windows-x64.exe; cd ..; $$env:GOOS='linux'; $$env:GOARCH='amd64'; go build -ldflags=\"$(LDFLAGS_LINUX)\" -o $(BUILDS_DIR)/$(APP_NAME)-linux-x64 $(SRC_DIR)"
else
	mkdir -p $(BUILDS_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS_WINDOWS)" -o $(BUILDS_DIR)/$(APP_NAME)-windows-x64.exe $(SRC_DIR)
	cd $(SRC_DIR) && go run github.com/tc-hib/go-winres@latest patch --no-backup ../$(BUILDS_DIR)/$(APP_NAME)-windows-x64.exe
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS_LINUX)" -o $(BUILDS_DIR)/$(APP_NAME)-linux-x64 $(SRC_DIR)
endif
