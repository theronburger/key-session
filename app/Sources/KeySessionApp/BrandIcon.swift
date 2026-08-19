import AppKit
import KeySessionKit
import SwiftUI

struct BrandIcon: View {
    var size: CGFloat = 30

    var body: some View {
        Image(nsImage: NSWorkspace.shared.icon(forFile: Bundle.main.bundlePath))
            .resizable()
            .interpolation(.high)
            .scaledToFit()
            .frame(width: size, height: size)
            .accessibilityLabel("Key Session")
    }
}

struct KeyMark: View {
    var size: CGFloat = 28

    var body: some View {
        Group {
            if let url = Bundle.main.url(forResource: "KeySessionKey", withExtension: "png"),
               let image = NSImage(contentsOf: url) {
                Image(nsImage: image)
                    .resizable()
                    .interpolation(.high)
                    .scaledToFit()
            }
        }
        .frame(width: size, height: size)
        .accessibilityHidden(true)
    }
}

struct MenuBarStatusLabel: View {
    @Bindable var model: KeySessionKit.AppModel

    var body: some View {
		if let lease = model.activeLeases.first {
			Label(model.activeLeases.count == 1 ? KeySessionKit.KeySessionFormat.remaining(until: lease.expiresAt) : "\(model.activeLeases.count)", systemImage: "key.fill")
		} else if !model.consumers.isEmpty {
			Label("\(model.consumers.count)", systemImage: "person.crop.circle")
        } else {
            Image(systemName: model.lifecycleState == .connected ? "key" : "key.slash")
        }
    }
}
