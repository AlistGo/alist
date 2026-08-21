import Foundation
import XCTest
@testable import AListTV

final class AListClientContractTests: XCTestCase {
    private var session: URLSession!

    override func setUp() {
        super.setUp()
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [ContractURLProtocol.self]
        session = URLSession(configuration: configuration)
    }

    override func tearDown() {
        session.invalidateAndCancel()
        ContractURLProtocol.handler = nil
        super.tearDown()
    }

    func testLoginRequestHeadersBodyAndBasePath() async throws {
        ContractURLProtocol.handler = { request in
            XCTAssertEqual(request.url?.absoluteString, "https://alist.example:8443/prefix/api/auth/login")
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Client-Id"), "stable-client")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Accept"), "application/json")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Content-Type"), "application/json")
            XCTAssertNil(request.value(forHTTPHeaderField: "Authorization"))
            let body = try Self.bodyData(for: request)
            let object = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
            XCTAssertEqual(object["username"], "alice")
            XCTAssertEqual(object["password"], "correct horse")
            XCTAssertNil(object["otp_code"])
            return Self.response(for: request, status: 200, body: Fixtures.loginSuccess)
        }
        let client = makeClient(baseURL: URL(string: "https://alist.example:8443/prefix")!)
        let data = try await client.login(username: "alice", password: "correct horse", otpCode: nil)
        XCTAssertEqual(data, LoginData(token: "secret-token", deviceKey: "device-key"))
    }

    func testMapsHTTP402ToOTPRequired() async {
        ContractURLProtocol.handler = { request in
            Self.response(for: request, status: 402, body: Fixtures.otpRequired)
        }
        let client = makeClient()
        await assertAPIError(.otpRequired) {
            _ = try await client.login(username: "alice", password: "password", otpCode: nil)
        }
    }

    func testMapsEnvelope401And429() async {
        let client = makeClient()
        ContractURLProtocol.handler = { request in
            Self.response(for: request, status: 200, body: "{\"code\":401,\"message\":\"expired\",\"data\":null}")
        }
        await assertAPIError(.unauthorized) { _ = try await client.currentUser() }

        ContractURLProtocol.handler = { request in
            Self.response(for: request, status: 429, body: "{\"code\":429,\"message\":\"slow down\",\"data\":null}")
        }
        await assertAPIError(.rateLimited) {
            _ = try await client.login(username: "alice", password: "password", otpCode: nil)
        }
    }

    func testDecodesOptionalListFieldsAndRawDates() async throws {
        ContractURLProtocol.handler = { request in
            XCTAssertEqual(request.url?.path, "/api/fs/list")
            return Self.response(for: request, status: 200, body: Fixtures.directoryWithOptionalFieldsMissing)
        }
        let page = try await makeClient().list(path: "/", page: 1, perPage: 200)
        XCTAssertNil(page.hasMore)
        XCTAssertNil(page.page)
        XCTAssertNil(page.perPage)
        XCTAssertNil(page.content[0].virtualPath)
        XCTAssertEqual(page.content[0].modified, "2026-08-19T10:11:12.123456789Z")
    }

    func testAuthenticatedHeaders() async throws {
        ContractURLProtocol.handler = { request in
            XCTAssertEqual(request.httpMethod, "GET")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "raw-token")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Client-Id"), "stable-client")
            return Self.response(for: request, status: 200, body: Fixtures.currentUser)
        }
        let user = try await makeClient().currentUser()
        XCTAssertEqual(user, CurrentUser(id: 7, username: "alice"))
    }

    private func makeClient(baseURL: URL = URL(string: "https://alist.example")!) -> AListClient {
        AListClient(baseURL: baseURL, clientID: "stable-client", tokenProvider: { "raw-token" }, session: session)
    }

    private func assertAPIError(
        _ expected: AListAPIError,
        operation: () async throws -> Void
    ) async {
        do {
            try await operation()
            XCTFail("Expected \(expected)")
        } catch let error as AListAPIError {
            XCTAssertEqual(error, expected)
        } catch {
            XCTFail("Unexpected error: \(error)")
        }
    }

    private static func bodyData(for request: URLRequest) throws -> Data {
        if let body = request.httpBody { return body }
        guard let stream = request.httpBodyStream else {
            throw AListAPIError.invalidResponse
        }
        stream.open()
        defer { stream.close() }
        var data = Data()
        var buffer = [UInt8](repeating: 0, count: 1_024)
        while true {
            let count = stream.read(&buffer, maxLength: buffer.count)
            if count < 0 { throw stream.streamError ?? AListAPIError.invalidResponse }
            if count == 0 { break }
            data.append(buffer, count: count)
        }
        return data
    }

    private static func response(for request: URLRequest, status: Int, body: String) -> (HTTPURLResponse, Data) {
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: status,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        return (response, Fixtures.data(body))
    }
}

private final class ContractURLProtocol: URLProtocol, @unchecked Sendable {
    static var handler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        do {
            let handler = try XCTUnwrap(Self.handler)
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}
