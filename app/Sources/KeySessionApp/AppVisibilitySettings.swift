import AppKit
import Foundation
import Observation

@MainActor
@Observable
final class AppVisibilitySettings {
    private enum PreferenceKey {
        static let showsMenuBar = "showsMenuBar"
        static let showsDockAndCommandTab = "showsDockAndCommandTab"
    }

    private(set) var showsMenuBar: Bool
    private(set) var showsDockAndCommandTab: Bool

    @ObservationIgnored private let preferences: UserDefaults

    init(preferences: UserDefaults = .standard) {
        self.preferences = preferences
        showsMenuBar = preferences.object(forKey: PreferenceKey.showsMenuBar) as? Bool ?? true
        showsDockAndCommandTab = preferences.object(forKey: PreferenceKey.showsDockAndCommandTab) as? Bool ?? true

        if !showsMenuBar && !showsDockAndCommandTab {
            showsMenuBar = true
        }

        persist()
    }

    func setMenuBarVisible(_ isVisible: Bool) {
        guard isVisible || showsDockAndCommandTab else { return }
        showsMenuBar = isVisible
        persist()
    }

    func setDockAndCommandTabVisible(_ isVisible: Bool) {
        guard isVisible || showsMenuBar else { return }
        showsDockAndCommandTab = isVisible
        persist()
        applyActivationPolicy()
    }

    func applyActivationPolicy() {
        NSApplication.shared.setActivationPolicy(showsDockAndCommandTab ? .regular : .accessory)
    }

    private func persist() {
        preferences.set(showsMenuBar, forKey: PreferenceKey.showsMenuBar)
        preferences.set(showsDockAndCommandTab, forKey: PreferenceKey.showsDockAndCommandTab)
    }
}
