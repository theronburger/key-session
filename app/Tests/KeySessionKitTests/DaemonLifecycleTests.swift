import Foundation
import XCTest
@testable import KeySessionKit

final class DaemonLifecycleTests: XCTestCase {
    func testLaunchAgentIsAttributedToKeySessionApp() {
        let propertyList = DaemonLifecycleController.launchAgentPropertyList(
            installedBinary: URL(fileURLWithPath: "/tmp/KeySessionDaemon"),
            logsDirectory: URL(fileURLWithPath: "/tmp/logs")
        )

        XCTAssertEqual(
            propertyList["AssociatedBundleIdentifiers"] as? [String],
            ["com.theronburger.key-session"]
        )
    }
}
