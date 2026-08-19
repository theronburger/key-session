import KeySessionKit
import SwiftUI

struct ActivityView: View {
    @Bindable var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Activity").font(.largeTitle.weight(.bold))
                    Text("Local access metadata only. Secrets and command arguments are never journaled.")
                        .font(.title3).foregroundStyle(.secondary)
                }
                if model.events.isEmpty {
                    EmptyPanel(title: "No activity yet", message: "Grants, revocations, expiries, and profile changes will appear here.", systemImage: "clock.arrow.circlepath")
                } else {
                    SectionCard {
                        ForEach(Array(model.events.enumerated()), id: \.element.id) { index, event in
                            ActivityRow(event: event)
                            if index < model.events.count - 1 { Divider() }
                        }
                    }
                }
            }
            .padding(30).frame(maxWidth: 980, alignment: .leading)
        }
    }
}

struct ActivityRow: View {
    let event: AuditEvent

    var body: some View {
        HStack(alignment: .top, spacing: 13) {
            Image(systemName: event.symbol).foregroundStyle(event.tint).frame(width: 22).padding(.top, 2)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 7) {
                    Text(event.displayTitle).font(.body.weight(.medium))
                    if let profile = event.profile { Text(profile).font(.caption.monospaced()).foregroundStyle(.secondary) }
                }
				if let consumer = event.consumerLabel, !consumer.isEmpty {
					Text("\(consumer) · \(event.reason ?? "No reason recorded")").font(.caption).foregroundStyle(.secondary).lineLimit(2)
                } else if let detail = event.detail {
                    Text(detail).font(.caption).foregroundStyle(.secondary)
                }
            }
            Spacer()
            TimelineView(.periodic(from: .now, by: 1)) { timeline in
                Text(KeySessionFormat.relative(event.occurredAt, now: timeline.date))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 6)
    }
}
