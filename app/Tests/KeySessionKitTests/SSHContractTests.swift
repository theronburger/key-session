import Foundation
import XCTest
@testable import KeySessionKit

final class SSHContractTests: XCTestCase {
    func testExistingSecretProfileStillDecodes() throws {
        let profile = try decodeProfile(#"{"name":"database","environment_variable":"DATABASE_URL","default_lease_seconds":3600}"#)
        XCTAssertFalse(profile.isSSH)
        XCTAssertEqual(profile.accessLabel, "DATABASE_URL")
        XCTAssertNil(profile.publicKey)
    }

    func testSSHProfileShowsSigningInsteadOfAnEnvironmentSecret() throws {
        let profile = try decodeProfile(#"{"name":"infra","kind":"ssh","public_key":"ssh-ed25519 public-fixture","environment_variable":"","default_lease_seconds":3600}"#)
        XCTAssertTrue(profile.isSSH)
        XCTAssertEqual(profile.accessLabel, "SSH signing")
        XCTAssertEqual(profile.publicKey, "ssh-ed25519 public-fixture")
    }

    func testHumanGrantDoesNotContainASecretOrConsumerCapability() throws {
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        let data = try encoder.encode(SSHGrantRequest(profile: "infra", durationSeconds: 900))
        let fields = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(fields["profile"] as? String, "infra")
        XCTAssertEqual(fields["duration_seconds"] as? Int, 900)
        XCTAssertEqual(Set(fields.keys), ["profile", "duration_seconds", "consumer_label", "reason"])
    }

    private func decodeProfile(_ json: String) throws -> KeyProfile {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(KeyProfile.self, from: Data(json.utf8))
    }
}
