import KeySessionKit
import SwiftUI

struct NewProfileSheet: View {
    @Bindable var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var isSSH = false
    @State private var environmentVariable = ""
    @State private var durationSeconds = 3600
    @State private var secret = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            HStack(spacing: 14) {
                BrandIcon(size: 48)
                VStack(alignment: .leading, spacing: 3) {
                    Text("New Keychain profile").font(.title2.weight(.semibold))
                    Text("Metadata is stored locally; the secret is stored in macOS Keychain.").foregroundStyle(.secondary)
                }
            }
            Form {
                Picker("Profile type", selection: $isSSH) {
                    Text("Secret for a program").tag(false)
                    Text("SSH signing identity").tag(true)
                }
                TextField("Profile name", text: $name, prompt: Text("production-read-only"))
                if !isSSH { TextField("Environment variable", text: $environmentVariable, prompt: Text("MONGODB_URI")) }
                Picker("Lease duration", selection: $durationSeconds) {
                    Text("15 minutes").tag(900); Text("30 minutes").tag(1800); Text("1 hour").tag(3600); Text("4 hours").tag(14400); Text("24 hours").tag(86400)
                }
                if isSSH {
                    Text("A new private key is generated inside the helper. Only its public key is available for copying.")
                } else {
                    SecureField("Secret", text: $secret)
                }
            }.formStyle(.grouped).scrollDisabled(true)
            HStack {
                Label(isSSH ? "Touch ID is required to unlock SSH signing." : "The daemon writes this value directly to Keychain.", systemImage: "lock.shield")
                    .font(.caption).foregroundStyle(.secondary)
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Save Profile") {
                    Task {
                        let saved: Bool
                        if isSSH { saved = await model.createSSHProfile(name: name, durationSeconds: durationSeconds) }
                        else { saved = await model.saveProfile(name: name, environmentVariable: environmentVariable, durationSeconds: durationSeconds, secret: secret) }
                        secret = ""
                        if saved { dismiss() }
                    }
                }
                .buttonStyle(.borderedProminent).keyboardShortcut(.defaultAction)
                .disabled(name.isEmpty || (!isSSH && (environmentVariable.isEmpty || secret.isEmpty)) || model.isWorking)
            }
        }
        .padding(24).frame(width: 560)
    }
}
