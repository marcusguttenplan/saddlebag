# Saddlebag — Build System
# Builds the macOS app (via xcodebuild) and the Go CLI (sb)

SCHEME       ?= Saddlebag
CONFIGURATION ?= Release
BUILD_DIR     = $(CURDIR)/build
APP_NAME      = Saddlebag.app
INSTALL_DIR   = /Applications
CLI_DIR       = $(CURDIR)/sb

.PHONY: all app cli install install-app install-cli clean help

## Build everything
all: app cli

## Build the macOS app (Release)
app:
	xcodebuild build \
		-project Saddlebag.xcodeproj \
		-scheme $(SCHEME) \
		-configuration $(CONFIGURATION) \
		-derivedDataPath $(BUILD_DIR) \
		CODE_SIGNING_ALLOWED=NO \
		ENABLE_APP_SANDBOX=NO
	@echo "✅ App built: $(BUILD_DIR)/Build/Products/$(CONFIGURATION)/$(APP_NAME)"

## Build the Go CLI
cli:
	$(MAKE) -C $(CLI_DIR) build
	@echo "✅ CLI built: $(CLI_DIR)/bin/sb"

## Install both app and CLI
install: install-app install-cli

## Copy app bundle to /Applications
install-app: app
	@echo "Installing $(APP_NAME) to $(INSTALL_DIR)..."
	rm -rf $(INSTALL_DIR)/$(APP_NAME)
	cp -R $(BUILD_DIR)/Build/Products/$(CONFIGURATION)/$(APP_NAME) $(INSTALL_DIR)/$(APP_NAME)
	@echo "✅ Installed to $(INSTALL_DIR)/$(APP_NAME)"

## Install sb CLI to /usr/local/bin
install-cli: cli
	$(MAKE) -C $(CLI_DIR) install
	@echo "✅ Installed sb to /usr/local/bin/sb"

## Clean all build artifacts
clean:
	rm -rf $(BUILD_DIR)
	$(MAKE) -C $(CLI_DIR) clean
	@echo "🧹 Cleaned"

## Show available targets
help:
	@echo "Saddlebag Build System"
	@echo ""
	@echo "  make all          Build app + CLI"
	@echo "  make app          Build macOS app (Release)"
	@echo "  make cli          Build Go CLI"
	@echo "  make install      Install app to /Applications + CLI to /usr/local/bin"
	@echo "  make install-app  Install app only"
	@echo "  make install-cli  Install CLI only"
	@echo "  make clean        Remove build artifacts"
