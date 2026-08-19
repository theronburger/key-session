import KeySessionKit
import SwiftUI

struct StatusDot: View {
    let color: Color

    var body: some View {
        Circle().fill(color).frame(width: 9, height: 9)
    }
}

struct SectionCard<Content: View>: View {
    let title: String?
    @ViewBuilder let content: Content

    init(_ title: String? = nil, @ViewBuilder content: () -> Content) {
        self.title = title
        self.content = content()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            if let title { Text(title).font(.headline) }
            content
        }
        .padding(18)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 13, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 13, style: .continuous).stroke(.separator.opacity(0.55)))
    }
}

struct Metric: View {
    let value: String
    let label: String
    var tint: Color = .primary

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(value).font(.title3.weight(.semibold)).foregroundStyle(tint).monospacedDigit()
            Text(label).font(.caption).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct EmptyPanel: View {
    let title: String
    let message: String
    let systemImage: String

    var body: some View {
        ContentUnavailableView(title, systemImage: systemImage, description: Text(message))
            .frame(maxWidth: .infinity, minHeight: 180)
    }
}

extension AuditEvent {
    var displayTitle: String {
		switch kind {
		case "consumer_started": "Consumer session started"
		case "consumer_ended": "Consumer session ended"
        case "granted": "Lease granted"
        case "revoked": "Lease revoked"
        case "expired": "Lease expired"
        case "profile_saved": "Profile saved"
        case "profile_removed": "Profile removed"
        case "profile_management_started": "Profile unlocked"
        case "profile_updated": "Profile updated"
        default: kind.replacingOccurrences(of: "_", with: " ").capitalized
        }
    }

    var symbol: String {
		switch kind {
		case "consumer_started": "person.crop.circle.badge.checkmark"
		case "consumer_ended": "person.crop.circle.badge.xmark"
        case "granted": "checkmark.shield.fill"
        case "revoked": "xmark.shield"
        case "expired": "clock.badge.exclamationmark"
        case "profile_saved": "key.horizontal.fill"
        case "profile_removed": "trash"
        case "profile_management_started": "lock.open.fill"
        case "profile_updated": "square.and.pencil"
        default: "info.circle"
        }
    }

    var tint: Color {
        switch kind {
		case "consumer_started", "granted", "profile_saved", "profile_management_started", "profile_updated": .green
		case "consumer_ended", "revoked", "profile_removed": .red
        case "expired": .secondary
        default: .secondary
        }
    }
}
