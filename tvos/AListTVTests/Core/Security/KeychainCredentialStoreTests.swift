import XCTest
@testable import AListTV

final class KeychainCredentialStoreTests: XCTestCase {
    private var store: KeychainCredentialStore!

    override func setUp() {
        super.setUp()
        store = KeychainCredentialStore(service: "com.alist.tv.tests.\(UUID().uuidString)")
    }

    override func tearDown() {
        try? store.deleteToken()
        store = nil
        super.tearDown()
    }

    func testMissingTokenIsNil() throws {
        XCTAssertNil(try store.loadToken())
    }

    func testSaveLoadUpdateAndDelete() throws {
        try store.saveToken("first")
        XCTAssertEqual(try store.loadToken(), "first")
        try store.saveToken("second")
        XCTAssertEqual(try store.loadToken(), "second")
        try store.deleteToken()
        XCTAssertNil(try store.loadToken())
    }
}
