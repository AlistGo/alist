import XCTest

final class AListTVUITests: XCTestCase {
    private var app: XCUIApplication!
    private let remote = XCUIRemote.shared

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
        app = XCUIApplication()
        let fixtureUsername = name.contains("OTPChallenge") ? "otp" : "alice"
        app.launchArguments.append(contentsOf: [
            "ui-testing",
            "ui-server=https://alist.example",
            "ui-username=\(fixtureUsername)",
            "ui-password=password",
            "ui-otp=123456"
        ])
        app.launch()
    }

    func testLoginThenBrowseRoot() {
        login(username: "alice")
        XCTAssertTrue(app.buttons["browser.item./Shows"].waitForExistence(timeout: 3))
        XCTAssertTrue(app.buttons["browser.item./Sample.mp4"].exists)
    }

    func testOTPChallengeThenLogin() {
        loginForm(username: "otp")
        focusAndSelect(app.buttons["connection.connect"], direction: .down)
        let otp = app.textFields["connection.otp"]
        XCTAssertTrue(otp.waitForExistence(timeout: 3))
        XCTAssertEqual(otp.value as? String, "123456")
        let verify = app.buttons["connection.verify"]
        if !verify.hasFocus { remote.press(.down) }
        if !verify.hasFocus { remote.press(.right) }
        XCTAssertTrue(verify.hasFocus)
        remote.press(.select)
        XCTAssertTrue(app.buttons["browser.item./Shows"].waitForExistence(timeout: 3))
    }

    func testLogoutReturnsToConnection() {
        login(username: "alice")
        let logout = app.buttons["browser.logout"]
        XCTAssertTrue(logout.waitForExistence(timeout: 3))
        focusAndSelect(logout, direction: .up)
        XCTAssertTrue(app.textFields["connection.server"].waitForExistence(timeout: 3))
        XCTAssertFalse(app.buttons["browser.item./Shows"].exists)
    }

    func testOpenVideoThenReturnRestoresFocus() {
        login(username: "alice")
        let video = app.buttons["browser.item./Sample.mp4"]
        XCTAssertTrue(video.waitForExistence(timeout: 3))
        focusAndSelect(video, direction: .right)
        let done = app.buttons["player.done"]
        XCTAssertTrue(done.waitForExistence(timeout: 3))
        focusAndSelect(done, direction: .down)
        XCTAssertTrue(video.waitForExistence(timeout: 3))
        expectation(for: NSPredicate(format: "value == 'focused'"), evaluatedWith: video)
        waitForExpectations(timeout: 3)
    }

    private func login(username: String) {
        loginForm(username: username)
        focusAndSelect(app.buttons["connection.connect"], direction: .down)
    }

    private func loginForm(username: String) {
        let server = app.textFields["connection.server"]
        XCTAssertTrue(server.waitForExistence(timeout: 3))
        XCTAssertEqual(server.value as? String, "https://alist.example")
        let usernameField = app.textFields["connection.username"]
        XCTAssertTrue(usernameField.exists)
        XCTAssertEqual(usernameField.value as? String, username)
        XCTAssertTrue(app.secureTextFields["connection.password"].exists)
    }

    private func focusAndSelect(
        _ element: XCUIElement,
        direction: XCUIRemote.Button
    ) {
        for candidate in [direction, .down, .up, .right, .left] {
            for _ in 0 ..< 8 where !element.hasFocus {
                remote.press(candidate)
            }
        }
        XCTAssertTrue(element.hasFocus, "Could not focus \(element)")
        remote.press(.select)
    }
}
