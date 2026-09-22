import KeySessionKit
import SwiftUI

struct ManageProfileSheet: View {
    @Bindable var model: AppModel
    let profile: KeyProfile

    @Environment(\.dismiss) private var dismiss
    @State private var environmentVariable: String
    @State private var durationSeconds: Int
    @State private var managementSession: ProfileManagementSession?
    @State private var secret = ""
    @State private var revealsSecret = false
    @State private var confirmsRemoval = false

    init(model: AppModel, profile: KeyProfile) {
        self.model = model
        self.profile = profile
        _environmentVariable = State(initialValue: profile.environmentVariable)
        _durationSeconds = State(initialValue: profile.defaultLeaseSeconds)
    }

    private var isUnlocking: Bool {
        model.authorizingProfileName == profile.name
    }

    private var isUnlocked: Bool {
        managementSession != nil && !secret.isEmpty
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            HStack(spacing: 14) {
                KeyMark(size: 48)
                VStack(alignment: .leading, spacing: 3) {
                    Text("Manage \(profile.name)").font(.title2.weight(.semibold))
                    Text(isUnlocked
                        ? "Touch ID approved a five-minute editing session."
                        : "The secret stays in Keychain until you click the eye.")
                        .foregroundStyle(.secondary)
                }
            }

            Form {
                LabeledContent("Profile name", value: profile.name)
                TextField("Environment variable", text: $environmentVariable)
                Picker("Lease duration", selection: $durationSeconds) {
                    Text("15 minutes").tag(900)
                    Text("30 minutes").tag(1800)
                    Text("1 hour").tag(3600)
                    Text("4 hours").tag(14400)
                    Text("24 hours").tag(86400)
                }
                LabeledContent("Secret") {
                    HStack(spacing: 8) {
                        Group {
                            if !isUnlocked {
                                Text("••••••••••••")
                                    .foregroundStyle(.secondary)
                            } else if revealsSecret {
                                TextField("", text: $secret)
                            } else {
                                SecureField("", text: $secret)
                            }
                        }
                        .textFieldStyle(.plain)
                        .privacySensitive()

                        Button(action: toggleSecretVisibility) {
                            if isUnlocking {
                                ProgressView().controlSize(.small)
                            } else {
                                Image(systemName: revealsSecret && isUnlocked ? "eye.slash" : "eye")
                            }
                        }
                        .buttonStyle(.plain)
                        .disabled(isUnlocking)
                        .accessibilityLabel(revealsSecret && isUnlocked ? "Hide secret" : "Authenticate and reveal secret")
                        .help(isUnlocked ? "Show or hide secret" : "Authenticate with Touch ID to reveal secret")
                    }
                }
            }
            .formStyle(.grouped)
            .scrollDisabled(true)

            Label(
                isUnlocked
                    ? "The secret exists only in this editor and is cleared when it closes."
                    : "Touch ID is required to reveal or save the secret. Removing a profile asks for separate approval.",
                systemImage: isUnlocked ? "lock.open.fill" : "lock.shield"
            )
            .font(.caption)
            .foregroundStyle(.secondary)

            HStack {
                Button("Remove Profile…", role: .destructive) { confirmsRemoval = true }
                    .disabled(model.isWorking)
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("Save Changes") { save() }
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut(.defaultAction)
                    .disabled(environmentVariable.isEmpty || !isUnlocked || model.isWorking)
            }
        }
        .padding(24)
        .frame(width: 590)
        .onDisappear { endSession() }
        .alert("Remove \(profile.name)?", isPresented: $confirmsRemoval) {
            Button("Cancel", role: .cancel) {}
            Button("Remove Profile", role: .destructive) { remove() }
        } message: {
            Text("This permanently removes the stored secret and profile, and ends its active leases. Touch ID approval is required next.")
        }
    }

    private func toggleSecretVisibility() {
        if isUnlocked {
            revealsSecret.toggle()
            return
        }
        Task {
            guard let session = await model.unlockProfileManagement(profile) else { return }
            managementSession = session
            secret = session.secret
            revealsSecret = true
        }
    }

    private func save() {
        guard let managementSession else { return }
        Task {
            let saved = await model.updateProfile(
                managementSession,
                environmentVariable: environmentVariable,
                durationSeconds: durationSeconds,
                secret: secret
            )
            secret = ""
            self.managementSession = nil
            if saved { dismiss() }
        }
    }

    private func remove() {
        Task {
            let removed = await model.delete(profile)
            if removed { dismiss() }
        }
    }

    private func endSession() {
        guard let managementSession else {
            secret = ""
            return
        }
        secret = ""
        self.managementSession = nil
        Task {
            await model.endProfileManagement(
                profileName: managementSession.profile.name,
                managementToken: managementSession.managementToken
            )
        }
    }
}
