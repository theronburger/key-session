import KeySessionKit
import SwiftUI

struct OverviewView: View {
    @Bindable var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                header
                leasePanel
                profileSection
                recentActivity
            }
            .padding(30)
            .frame(maxWidth: 1050, alignment: .leading)
        }
    }

    private var header: some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: 5) {
                Text("Profiles")
                    .font(.largeTitle.weight(.bold))
                Text("Keychain-backed credentials, human management, and active access.")
                    .font(.title3).foregroundStyle(.secondary)
            }
            Spacer()
            Button { model.showsNewProfile = true } label: {
                Label("New Profile", systemImage: "plus")
            }
            .buttonStyle(.bordered)
        }
    }

	@ViewBuilder private var leasePanel: some View {
		if !model.consumers.isEmpty {
			VStack(alignment: .leading, spacing: 11) {
				HStack {
					Text("Consumer sessions").font(.title2.weight(.semibold))
					Text("\(model.consumers.count)").font(.caption.weight(.bold)).foregroundStyle(.secondary)
						.padding(6).background(.secondary.opacity(0.1), in: Circle())
					Spacer()
				}
				ForEach(model.consumers) { consumer in
					TimelineView(.periodic(from: .now, by: 1)) { timeline in
						SectionCard {
							HStack(spacing: 13) {
								ZStack {
									Circle().fill(.green.opacity(0.13)).frame(width: 44, height: 44)
									Image(systemName: "person.crop.circle.badge.checkmark").foregroundStyle(.green)
								}
								VStack(alignment: .leading, spacing: 2) {
									Text(consumer.label).font(.headline)
									Text("Consumer session · \(consumer.leases.count) lease\(consumer.leases.count == 1 ? "" : "s")")
										.font(.caption).foregroundStyle(.secondary)
								}
								Spacer()
								VStack(alignment: .trailing, spacing: 1) {
									Text(KeySessionFormat.remaining(until: consumer.expiresAt, now: timeline.date)).monospacedDigit()
									Text("session remaining").font(.caption2).foregroundStyle(.secondary)
								}
								Button("End Session", role: .destructive) {
									Task { await model.revoke(consumerId: consumer.id) }
								}.disabled(model.isWorking)
							}
							Divider()
							if consumer.leases.isEmpty {
								Label("No active leases. New grants can still reuse this task capability.", systemImage: "lock.fill")
									.font(.caption).foregroundStyle(.secondary)
							} else {
								ForEach(Array(consumer.leases.enumerated()), id: \.element.id) { index, lease in
									LeaseAccessRow(lease: lease, now: timeline.date) {
										Task { await model.revoke(consumerId: consumer.id, leaseId: lease.id) }
									}
									.disabled(model.isWorking)
									if index < consumer.leases.count - 1 { Divider() }
								}
							}
						}
					}
				}
			}
		} else {
            SectionCard {
                HStack(spacing: 18) {
                    ZStack {
                        Circle().fill(.secondary.opacity(0.10)).frame(width: 58, height: 58)
                        Image(systemName: "lock.fill").font(.title2).foregroundStyle(.secondary)
                    }
                    VStack(alignment: .leading, spacing: 5) {
						Text("No active leases").font(.title2.weight(.semibold))
						Text("Each CLI or MCP task gets its own capability-isolated consumer session.")
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    Text("SECURE").font(.caption2.weight(.bold)).foregroundStyle(.secondary)
                        .padding(.horizontal, 8).padding(.vertical, 4).background(.secondary.opacity(0.1), in: Capsule())
                }
            }
        }
    }

    private var profileSection: some View {
        VStack(alignment: .leading, spacing: 11) {
            HStack {
                Text("Stored profiles").font(.title2.weight(.semibold))
                Text("\(model.profiles.count)").font(.caption.weight(.bold)).foregroundStyle(.secondary)
                    .padding(6).background(.secondary.opacity(0.1), in: Circle())
                Spacer()
            }
            if model.profiles.isEmpty {
                EmptyPanel(title: "No profiles yet", message: "Create a profile to store a secret in macOS Keychain.", systemImage: "key.horizontal")
            } else {
                VStack(spacing: 0) {
                    ForEach(model.profiles) { profile in
                        ProfileRow(profile: profile) {
                            model.presentedProfileEditor = profile
                        }
                        if profile.id != model.profiles.last?.id { Divider().padding(.leading, 48) }
                    }
                }
                .padding(.horizontal, 14)
                .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 12))
                .overlay(RoundedRectangle(cornerRadius: 12).stroke(.separator.opacity(0.55)))
            }
        }
    }

    @ViewBuilder private var recentActivity: some View {
        if !model.events.isEmpty {
            VStack(alignment: .leading, spacing: 11) {
                HStack { Text("Recent activity").font(.title2.weight(.semibold)); Spacer(); Button("View All") { model.selection = .activity } }
                SectionCard {
                    ForEach(Array(model.events.prefix(3).enumerated()), id: \.element.id) { index, event in
                        ActivityRow(event: event)
                        if index < min(model.events.count, 3) - 1 { Divider() }
                    }
                }
            }
        }
    }
}

private struct LeaseAccessRow: View {
	let lease: KeyLease
	let now: Date
	let revoke: () -> Void

	var body: some View {
		HStack(alignment: .center, spacing: 12) {
			Image(systemName: "key.fill").foregroundStyle(.green).frame(width: 22)
			VStack(alignment: .leading, spacing: 2) {
				HStack(spacing: 7) {
					Text(lease.profile).font(.body.weight(.medium))
					Text(lease.environmentVariable).font(.caption.monospaced()).foregroundStyle(.secondary)
				}
				Text(lease.reason).font(.caption).foregroundStyle(.secondary).lineLimit(2)
			}
			Spacer()
			VStack(alignment: .trailing, spacing: 1) {
				Text(KeySessionFormat.remaining(until: lease.expiresAt, now: now)).font(.body.weight(.semibold)).monospacedDigit()
				Text(lease.id).font(.caption2.monospaced()).foregroundStyle(.tertiary)
			}
			Button("Revoke", role: .destructive, action: revoke).buttonStyle(.bordered).controlSize(.small)
		}
		.padding(.vertical, 3)
	}
}

struct ProfileRow: View {
    let profile: KeyProfile
    let edit: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            KeyMark(size: 29)
                .rotationEffect(.degrees(90))
                .frame(width: 34)
            VStack(alignment: .leading, spacing: 2) {
                Text(profile.name).font(.body.weight(.medium))
                Text("\(profile.environmentVariable) · \(KeySessionFormat.duration(profile.defaultLeaseSeconds))")
                    .font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            Button(action: edit) {
                Image(systemName: "square.and.pencil")
            }
            .buttonStyle(.bordered)
            .controlSize(.small)
            .accessibilityLabel("Edit \(profile.name)")
            .help("Edit \(profile.name)")
        }
        .padding(.vertical, 11)
    }
}
