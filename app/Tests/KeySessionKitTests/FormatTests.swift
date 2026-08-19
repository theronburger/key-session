import XCTest
@testable import KeySessionKit

final class FormatTests: XCTestCase {
    func testDurationFormatting() {
        XCTAssertEqual(KeySessionFormat.duration(900), "15m")
        XCTAssertEqual(KeySessionFormat.duration(3600), "1h")
        XCTAssertEqual(KeySessionFormat.duration(5400), "1h 30m")
    }
}
