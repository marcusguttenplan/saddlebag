import AppKit

/// Generates the menubar icon and app icon using SF Symbols
enum SaddleIcon {
    static func menuBarImage() -> NSImage {
        let config = NSImage.SymbolConfiguration(pointSize: 14, weight: .medium)
        let image = NSImage(systemSymbolName: "briefcase.fill", accessibilityDescription: "Saddlebag")!
            .withSymbolConfiguration(config)!
        image.isTemplate = true
        return image
    }

    /// Renders a polished app icon from an SF Symbol at the given size
    static func appIcon(size: CGFloat = 512) -> NSImage {
        let image = NSImage(size: NSSize(width: size, height: size))
        image.lockFocus()

        // Background: rounded rect with gradient
        let rect = NSRect(origin: .zero, size: NSSize(width: size, height: size))
        let cornerRadius = size * 0.22
        let path = NSBezierPath(roundedRect: rect, xRadius: cornerRadius, yRadius: cornerRadius)

        let gradient = NSGradient(
            starting: NSColor(red: 0.30, green: 0.50, blue: 0.85, alpha: 1.0),
            ending: NSColor(red: 0.15, green: 0.30, blue: 0.65, alpha: 1.0)
        )!
        gradient.draw(in: path, angle: -45)

        // SF Symbol centered
        let symbolConfig = NSImage.SymbolConfiguration(pointSize: size * 0.40, weight: .medium)
            .applying(.init(paletteColors: [.white]))
        if let symbol = NSImage(systemSymbolName: "briefcase.fill", accessibilityDescription: "Saddlebag")?
            .withSymbolConfiguration(symbolConfig) {
            let symbolSize = symbol.size
            let origin = NSPoint(
                x: (size - symbolSize.width) / 2,
                y: (size - symbolSize.height) / 2
            )
            symbol.draw(at: origin, from: .zero, operation: .sourceOver, fraction: 1.0)
        }

        image.unlockFocus()
        return image
    }
}
