import Foundation
import XCTest
@testable import AListTV

final class PlaybackProgressStoreTests: XCTestCase {
    private var defaults: UserDefaults!
    private var store: PlaybackProgressStore!

    override func setUp() {
        super.setUp()
        let suite = "PlaybackProgressStoreTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)!
        defaults.removePersistentDomain(forName: suite)
        store = PlaybackProgressStore(defaults: defaults)
    }

    override func tearDown() {
        defaults = nil
        store = nil
        super.tearDown()
    }

    func testThirtySecondThreshold() {
        let id = identity(path: "/movie.mp4")
        store.update(identity: id, position: 29.999, duration: 200)
        XCTAssertNil(store.record(for: id))
        store.update(identity: id, position: 30, duration: 200)
        XCTAssertEqual(store.record(for: id)?.position, 30)
    }

    func testNinetyPercentDeletesRecord() {
        let id = identity(path: "/movie.mp4")
        store.update(identity: id, position: 50, duration: 100)
        XCTAssertNotNil(store.record(for: id))
        store.update(identity: id, position: 90, duration: 100)
        XCTAssertNil(store.record(for: id))
    }

    func testInvalidDurationDoesNotSave() {
        let id = identity(path: "/movie.mp4")
        store.update(identity: id, position: 50, duration: 0)
        store.update(identity: id, position: 50, duration: .infinity)
        store.update(identity: id, position: 50, duration: .nan)
        XCTAssertNil(store.record(for: id))
    }

    func testRetainsAtMostFiveHundredNewestRecords() {
        for index in 0 ..< 501 {
            store.update(identity: identity(path: "/\(index).mp4"), position: 30, duration: 100)
        }
        XCTAssertEqual(store.allRecords().count, 500)
    }

    func testServerUserAndPathAreIndependentIdentityFields() {
        let base = identity(path: "/movie.mp4")
        let otherServer = PlaybackProgressIdentity(baseURL: "https://other.example", username: "alice", virtualPath: "/movie.mp4")
        let otherUser = PlaybackProgressIdentity(baseURL: "https://alist.example", username: "bob", virtualPath: "/movie.mp4")
        let otherPath = identity(path: "/other.mp4")
        store.update(identity: base, position: 31, duration: 100)
        store.update(identity: otherServer, position: 32, duration: 100)
        store.update(identity: otherUser, position: 33, duration: 100)
        store.update(identity: otherPath, position: 34, duration: 100)

        XCTAssertEqual(store.record(for: base)?.position, 31)
        XCTAssertEqual(store.record(for: otherServer)?.position, 32)
        XCTAssertEqual(store.record(for: otherUser)?.position, 33)
        XCTAssertEqual(store.record(for: otherPath)?.position, 34)
    }

    private func identity(path: String) -> PlaybackProgressIdentity {
        PlaybackProgressIdentity(baseURL: "https://alist.example", username: "alice", virtualPath: path)
    }
}
