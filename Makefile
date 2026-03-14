APP_NAME := Saddlebag
BUNDLE := $(APP_NAME).app
INSTALL_DIR := /Applications
BUILD_DIR := .build/release

.PHONY: build bundle install run clean

# Build release binary
build:
	swift build -c release

# Create .app bundle from release build
bundle: build
	@echo "→ Creating $(BUNDLE)..."
	@mkdir -p $(BUNDLE)/Contents/MacOS
	@mkdir -p $(BUNDLE)/Contents/Resources
	@cp $(BUILD_DIR)/$(APP_NAME) $(BUNDLE)/Contents/MacOS/
	@cp $(APP_NAME)/Info.plist $(BUNDLE)/Contents/
	@echo "✓ $(BUNDLE) created"

# Install to /Applications (overwrites existing)
install: bundle
	@echo "→ Installing to $(INSTALL_DIR)..."
	@rm -rf $(INSTALL_DIR)/$(BUNDLE)
	@cp -r $(BUNDLE) $(INSTALL_DIR)/
	@echo "✓ Installed to $(INSTALL_DIR)/$(BUNDLE)"

# Build, install, and launch
run: install
	@echo "→ Launching $(APP_NAME)..."
	@open $(INSTALL_DIR)/$(BUNDLE)

# Remove build artifacts and local bundle
clean:
	swift package clean
	@rm -rf $(BUNDLE)
	@echo "✓ Cleaned"
