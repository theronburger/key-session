import KeySessionKit
import SwiftUI

struct SSHProfileSheet: View {
    @Bindable var model: AppModel
    let profile: KeyProfile
    @Environment(\.dismiss) private var dismiss
    @State private var durationSeconds: Int
    @State private var confirmsRemoval = false

    init(model: AppModel, profile: KeyProfile) {
        self.model = model
        self.profile = profile
        _durationSeconds = State(initialValue: profile.defaultLeaseSeconds)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Text(profile.name).font(.title2.weight(.semibold))
            Text("Approve SSH access with Touch ID. The private key stays inside Key Session.")
                .foregroundStyle(.secondary)
            Form {
                LabeledContent("Public key") {
                    Text(profile.publicKey ?? "Unavailable")
                        .font(.caption.monospaced()).textSelection(.enabled)
                }
                Picker("Access duration", selection: $durationSeconds) {
                    Text("15 minutes").tag(900)
                    Text("30 minutes").tag(1800)
                    Text("1 hour").tag(3600)
                    Text("4 hours").tag(14400)
                    Text("24 hours").tag(86400)
                }
            }
            .formStyle(.grouped).scrollDisabled(true)
            Text("While any lease for this identity is active, programs under your Mac account can use it. Expiry or revocation prevents new logins; existing connections stay open. Locking the screen does not end a lease.")
                .font(.caption).foregroundStyle(.secondary)
            HStack {
                Button("Remove Profile…", role: .destructive) { confirmsRemoval = true }
                    .disabled(model.isWorking)
                Spacer()
                Button("Close") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Approve SSH Access") {
                    Task {
                        if await model.grantSSH(profile, durationSeconds: durationSeconds) { dismiss() }
                    }
                }
                .buttonStyle(.borderedProminent).disabled(model.isWorking)
            }
        }
        .padding(24).frame(width: 590)
        .alert("Remove \(profile.name)?", isPresented: $confirmsRemoval) {
            Button("Cancel", role: .cancel) {}
            Button("Remove Profile", role: .destructive) {
                Task { if await model.delete(profile) { dismiss() } }
            }
        } message: {
            Text("This removes the private key and ends its leases after Touch ID approval. Servers still trusting its public key must be updated separately. This cannot be undone.")
        }
    }
}
