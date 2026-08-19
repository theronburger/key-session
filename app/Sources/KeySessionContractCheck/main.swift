import Foundation
import KeySessionKit

do {
    let client = try DaemonClient.connect()
    let snapshot = try await client.snapshot()
    print("schema=\(snapshot.schemaVersion) revision=\(snapshot.revision) profiles=\(snapshot.profiles.count) daemon=\(snapshot.daemon.version)")
} catch {
    fputs("contract check failed: \(error)\n", stderr)
    exit(1)
}
