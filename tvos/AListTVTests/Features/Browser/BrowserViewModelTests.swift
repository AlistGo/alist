import Foundation
import XCTest
@testable import AListTV

final class AListPathTests: XCTestCase {
    func testPOSIXJoinAndRootNormalization() throws {
        XCTAssertEqual(try AListPath.join(parent: "/", name: "Shows"), "/Shows")
        XCTAssertEqual(try AListPath.join(parent: "//Media///Shows/", name: "Season 1"), "/Media/Shows/Season 1")
        XCTAssertEqual(AListPath.parent(of: "/Media/Shows"), "/Media")
        XCTAssertEqual(AListPath.parent(of: "/Media"), "/")
    }

    func testRejectsSlashInName() {
        XCTAssertThrowsError(try AListPath.join(parent: "/", name: "nested/name"))
    }
}

@MainActor
final class BrowserViewModelTests: XCTestCase {
    func testRootRequestAndStableDirectoryPartition() async throws {
        let page = DirectoryPage(content: [
            object("file-a", directory: false),
            object("dir-a", directory: true),
            object("file-b", directory: false),
            object("dir-b", directory: true)
        ], hasMore: false, page: 1, perPage: 200)
        let api = BrowserFakeAPI(responses: [.success(path: "/", page: 1, value: page)])
        let viewModel = BrowserViewModel(api: api)

        viewModel.loadInitial()
        try await waitUntil { viewModel.state == .loaded }

        XCTAssertEqual(viewModel.items.map(\.name), ["dir-a", "dir-b", "file-a", "file-b"])
        let calls = await api.calls
        XCTAssertEqual(calls, [ListCall(path: "/", page: 1, perPage: 200)])
    }

    func testPaginationDeduplicates() async throws {
        let first = DirectoryPage(content: [object("a", directory: true), object("b", directory: false)], hasMore: true, page: 1, perPage: 2)
        let second = DirectoryPage(content: [object("b", directory: false), object("c", directory: true)], hasMore: false, page: 2, perPage: 2)
        let api = BrowserFakeAPI(responses: [
            .success(path: "/", page: 1, value: first),
            .success(path: "/", page: 2, value: second)
        ])
        let viewModel = BrowserViewModel(api: api, perPage: 2)
        viewModel.loadInitial()
        try await waitUntil { viewModel.items.count == 2 }

        viewModel.loadNextPageIfNeeded(currentItem: viewModel.items[0])
        try await waitUntil { viewModel.items.count == 3 }

        XCTAssertEqual(viewModel.items.map(\.name), ["a", "c", "b"])
        XCTAssertFalse(viewModel.hasMore)
    }

    func testLegacyHasMoreFallback() async throws {
        let api = BrowserFakeAPI(responses: [
            .success(path: "/", page: 1, value: DirectoryPage(content: [object("a"), object("b")], hasMore: nil, page: nil, perPage: nil)),
            .success(path: "/", page: 2, value: DirectoryPage(content: [], hasMore: nil, page: nil, perPage: nil))
        ])
        let viewModel = BrowserViewModel(api: api, perPage: 2)
        viewModel.loadInitial()
        try await waitUntil { viewModel.state == .loaded && viewModel.hasMore }
        viewModel.loadNextPageIfNeeded(currentItem: viewModel.items.last!)
        try await waitUntil { viewModel.state == .loaded && !viewModel.hasMore }

        let calls = await api.calls
        XCTAssertEqual(calls.map(\.page), [1, 2])
    }

    func testConcurrentThresholdRequestsOnce() async throws {
        let api = BrowserFakeAPI(responses: [
            .success(path: "/", page: 1, value: DirectoryPage(content: [object("a"), object("b")], hasMore: true, page: 1, perPage: 2)),
            .success(path: "/", page: 2, value: DirectoryPage(content: [object("c")], hasMore: false, page: 2, perPage: 2), delay: 150_000_000)
        ])
        let viewModel = BrowserViewModel(api: api, perPage: 2)
        viewModel.loadInitial()
        try await waitUntil { viewModel.items.count == 2 }
        let item = viewModel.items.last!

        viewModel.loadNextPageIfNeeded(currentItem: item)
        viewModel.loadNextPageIfNeeded(currentItem: item)
        viewModel.loadNextPageIfNeeded(currentItem: item)
        try await waitUntil { viewModel.items.count == 3 }

        let calls = await api.calls
        XCTAssertEqual(calls.filter { $0.page == 2 }.count, 1)
    }

    func testPathChangeIgnoresStaleResponse() async throws {
        let folder = object("Folder", directory: true)
        let api = BrowserFakeAPI(responses: [
            .success(path: "/", page: 1, value: DirectoryPage(content: [folder], hasMore: false, page: 1, perPage: 200)),
            .success(path: "/Folder", page: 1, value: DirectoryPage(content: [object("stale")], hasMore: false, page: 1, perPage: 200), delay: 200_000_000),
            .success(path: "/", page: 1, value: DirectoryPage(content: [folder, object("fresh")], hasMore: false, page: 1, perPage: 200))
        ])
        let viewModel = BrowserViewModel(api: api)
        viewModel.loadInitial()
        try await waitUntil { viewModel.items.count == 1 }
        viewModel.open(viewModel.items[0])
        viewModel.moveToParent()
        try await waitUntil { viewModel.items.count == 2 }

        XCTAssertEqual(viewModel.path, "/")
        XCTAssertEqual(viewModel.items.map(\.name), ["Folder", "fresh"])
    }

    func testReturnRestoresFocus() async throws {
        let folder = object("Folder", directory: true)
        let api = BrowserFakeAPI(responses: [
            .success(path: "/", page: 1, value: DirectoryPage(content: [folder], hasMore: false, page: 1, perPage: 200)),
            .success(path: "/Folder", page: 1, value: DirectoryPage(content: [], hasMore: false, page: 1, perPage: 200)),
            .success(path: "/", page: 1, value: DirectoryPage(content: [folder], hasMore: false, page: 1, perPage: 200))
        ])
        let viewModel = BrowserViewModel(api: api)
        viewModel.loadInitial()
        try await waitUntil { viewModel.items.count == 1 }
        viewModel.open(viewModel.items[0])
        try await waitUntil { viewModel.state == .empty }
        viewModel.moveToParent()
        try await waitUntil { viewModel.focusedVirtualPath == "/Folder" }

        XCTAssertEqual(viewModel.focusedVirtualPath, "/Folder")
    }

    func testForbiddenRetryState() async throws {
        let api = BrowserFakeAPI(responses: [
            .failure(path: "/", page: 1, error: .server(code: 403, message: "forbidden")),
            .success(path: "/", page: 1, value: DirectoryPage(content: [object("ok")], hasMore: false, page: 1, perPage: 200))
        ])
        let viewModel = BrowserViewModel(api: api)
        viewModel.loadInitial()
        try await waitUntil { viewModel.state == .forbidden }
        viewModel.retry()
        try await waitUntil { viewModel.items.count == 1 }
        XCTAssertEqual(viewModel.state, .loaded)
    }

    private func object(_ name: String, directory: Bool = false) -> AListObject {
        AListObject(virtualPath: "/\(name)", name: name, isDirectory: directory)
    }

    private func waitUntil(
        timeoutNanoseconds: UInt64 = 2_000_000_000,
        condition: @escaping @MainActor () -> Bool
    ) async throws {
        let started = ContinuousClock.now
        while !condition() {
            if ContinuousClock.now - started > .nanoseconds(Int64(timeoutNanoseconds)) {
                throw AListAPIError.transport(message: "Timed out waiting for browser state")
            }
            try await Task.sleep(for: .milliseconds(10))
        }
    }
}

private struct ListCall: Equatable, Sendable {
    let path: String
    let page: Int
    let perPage: Int
}

private struct BrowserResponse: Sendable {
    let path: String
    let page: Int
    let result: Result<DirectoryPage, AListAPIError>
    let delay: UInt64

    static func success(path: String, page: Int, value: DirectoryPage, delay: UInt64 = 0) -> BrowserResponse {
        BrowserResponse(path: path, page: page, result: .success(value), delay: delay)
    }

    static func failure(path: String, page: Int, error: AListAPIError) -> BrowserResponse {
        BrowserResponse(path: path, page: page, result: .failure(error), delay: 0)
    }
}

private actor BrowserFakeAPI: AListAPI {
    private var responses: [BrowserResponse]
    private(set) var calls: [ListCall] = []

    init(responses: [BrowserResponse]) { self.responses = responses }

    func login(username: String, password: String, otpCode: String?) async throws -> LoginData {
        throw AListAPIError.invalidResponse
    }

    func currentUser() async throws -> CurrentUser { throw AListAPIError.invalidResponse }

    func list(path: String, page: Int, perPage: Int) async throws -> DirectoryPage {
        calls.append(ListCall(path: path, page: page, perPage: perPage))
        guard let index = responses.firstIndex(where: { $0.path == path && $0.page == page }) else {
            throw AListAPIError.invalidResponse
        }
        let response = responses.remove(at: index)
        if response.delay > 0 { try? await Task.sleep(nanoseconds: response.delay) }
        return try response.result.get()
    }

    func get(path: String) async throws -> FileDetail { throw AListAPIError.invalidResponse }
}
