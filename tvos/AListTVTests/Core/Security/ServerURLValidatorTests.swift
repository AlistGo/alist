import XCTest
@testable import AListTV

final class ServerURLValidatorTests: XCTestCase {
    func testAcceptsHTTPSAndNormalizesTrailingSlashes() throws {
        XCTAssertEqual(
            try ServerURLValidator.validate("  https://alist.example:8443/prefix///  ").absoluteString,
            "https://alist.example:8443/prefix"
        )
        XCTAssertEqual(
            try ServerURLValidator.validate("https://alist.example/").absoluteString,
            "https://alist.example"
        )
    }

    func testRejectsHTTPAndRelativeURLs() {
        assertError(.insecureURL, value: "http://alist.example")
        assertError(.invalidServerURL, value: "/relative/path")
        assertError(.invalidServerURL, value: "alist.example")
    }

    func testRejectsMissingHostCredentialsQueryAndFragment() {
        assertError(.invalidServerURL, value: "https:///path")
        assertError(.invalidServerURL, value: "https://user:password@alist.example")
        assertError(.invalidServerURL, value: "https://alist.example?query=value")
        assertError(.invalidServerURL, value: "https://alist.example/#fragment")
    }

    private func assertError(_ expected: AListAPIError, value: String) {
        XCTAssertThrowsError(try ServerURLValidator.validate(value)) { error in
            XCTAssertEqual(error as? AListAPIError, expected)
        }
    }
}
