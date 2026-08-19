import KeySessionKit
import SwiftUI

struct SettingsView: View {
    @Bindable var model: AppModel
    @Bindable var updates: AppUpdateController

    var body: some View {
        Form {
            Section("Daemon") {
                LabeledContent("Status", value: model.lifecycleState.title)
                LabeledContent("Architecture", value: "App · CLI · MCP → daemon")
                Button("Repair Installation") { Task { await model.repair() } }
            }
            Section("Updates") {
                LabeledContent("Installed version", value: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "Development")
                if let availableVersion = updates.availableVersion {
                    LabeledContent("Available version", value: availableVersion)
                }
                Button(updates.buttonTitle) { updates.checkForUpdates() }
                    .disabled(!updates.canCheckForUpdates)
                Toggle("Check automatically", isOn: Binding(
                    get: { updates.automaticallyChecksForUpdates },
                    set: { updates.setAutomaticUpdateChecks($0) }
                ))
                Text("Updates are checked automatically and verified with Key Session's Ed25519 release key before installation.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Section("Privacy") {
				Text("The UI receives consumer labels, profile names, lease timing, and audit metadata. Consumer capabilities, active secrets, and command output never enter the UI.")
                    .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped).padding().frame(width: 520, height: 390)
    }
}
