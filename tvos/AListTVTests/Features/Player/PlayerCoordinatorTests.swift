import Foundation
import XCTest
@testable import AListTV

@MainActor
final class PlayerCoordinatorTests: XCTestCase {
    func testGetPrecedesItemCreation() async {
        let log = LockedEventLog()
        let api = PlayerFakeAPI(details: [detail(url: "https://media.example/video.mp4", path: "/video.mp4")], log: log)
        let player = PlayerFakeController(log: log)
        let coordinator = makeCoordinator(api: api, player: player)

        await coordinator.play(object: object("/video.mp4"))

        XCTAssertEqual(log.values, ["get:/video.mp4", "replace:https://media.example/video.mp4"])
        XCTAssertEqual(player.playCount, 1)
        XCTAssertEqual(coordinator.state, .playing)
    }

    func testRejectsInsecureAndEmptyRawURL() async {
        let api = PlayerFakeAPI(details: [
            detail(url: "http://media.example/video.mp4", path: "/one.mp4"),
            detail(url: "", path: "/two.mp4")
        ])
        let player = PlayerFakeController()
        let coordinator = makeCoordinator(api: api, player: player)

        await coordinator.play(object: object("/one.mp4"))
        guard case .failed = coordinator.state else { return XCTFail("HTTP URL was accepted") }
        XCTAssertTrue(coordinator.isPresented)
        coordinator.playerDidDisappear()
        await coordinator.play(object: object("/two.mp4"))
        guard case .failed = coordinator.state else { return XCTFail("Empty URL was accepted") }
        XCTAssertTrue(coordinator.isPresented)
        XCTAssertTrue(player.replacedURLs.isEmpty)
    }

    func testDirectoryDoesNotRequestGet() async {
        let api = PlayerFakeAPI(details: [])
        let coordinator = makeCoordinator(api: api, player: PlayerFakeController())
        await coordinator.play(object: AListObject(virtualPath: "/Folder", name: "Folder", isDirectory: true))
        let paths = await api.getPaths
        XCTAssertEqual(paths, [])
    }

    func testFirstFailureRefreshesAndRestores() async throws {
        let api = PlayerFakeAPI(details: [
            detail(url: "https://media.example/first", path: "/video.mp4"),
            detail(url: "https://media.example/refreshed", path: "/video.mp4")
        ])
        let player = PlayerFakeController()
        player.currentTime = 42
        let coordinator = makeCoordinator(api: api, player: player)
        await coordinator.play(object: object("/video.mp4"))

        player.emit(.failed(message: "expired"))
        try await waitUntil { player.replacedURLs.count == 2 }

        XCTAssertEqual(player.replacedURLs.map(\.absoluteString), [
            "https://media.example/first",
            "https://media.example/refreshed"
        ])
        XCTAssertEqual(player.seekValues.last, 42)
        let paths = await api.getPaths
        XCTAssertEqual(paths.count, 2)
    }

    func testSecondFailureDoesNotRefresh() async throws {
        let api = PlayerFakeAPI(details: [
            detail(url: "https://media.example/first", path: "/video.mp4"),
            detail(url: "https://media.example/refreshed", path: "/video.mp4")
        ])
        let player = PlayerFakeController()
        let coordinator = makeCoordinator(api: api, player: player)
        await coordinator.play(object: object("/video.mp4"))
        player.emit(.failed(message: "first"))
        try await waitUntil { player.replacedURLs.count == 2 }
        player.emit(.failed(message: "second underlying failure"))
        try await waitUntil { coordinator.state == .failed(message: "second underlying failure") }

        let paths = await api.getPaths
        XCTAssertEqual(paths.count, 2)
    }

    func testNewObjectResetsRetryBudget() async throws {
        let api = PlayerFakeAPI(details: [
            detail(url: "https://media.example/a1", path: "/a.mp4"),
            detail(url: "https://media.example/a2", path: "/a.mp4"),
            detail(url: "https://media.example/b1", path: "/b.mp4"),
            detail(url: "https://media.example/b2", path: "/b.mp4")
        ])
        let player = PlayerFakeController()
        let coordinator = makeCoordinator(api: api, player: player)
        await coordinator.play(object: object("/a.mp4"))
        player.emit(.failed(message: "a"))
        try await waitUntil { player.replacedURLs.count == 2 }
        await coordinator.play(object: object("/b.mp4"))
        player.emit(.failed(message: "b"))
        try await waitUntil { player.replacedURLs.count == 4 }

        let paths = await api.getPaths
        XCTAssertEqual(paths, ["/a.mp4", "/a.mp4", "/b.mp4", "/b.mp4"])
    }

    func testRestoresSavedProgressBeforePlay() async {
        let suite = "PlayerCoordinatorTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defaults.removePersistentDomain(forName: suite)
        let store = PlaybackProgressStore(defaults: defaults)
        let identity = PlaybackProgressIdentity(baseURL: "https://alist.example", username: "alice", virtualPath: "/video.mp4")
        store.update(identity: identity, position: 60, duration: 200)
        let api = PlayerFakeAPI(details: [detail(url: "https://media.example/video", path: "/video.mp4")])
        let player = PlayerFakeController()
        let coordinator = makeCoordinator(api: api, player: player, store: store)

        await coordinator.play(object: object("/video.mp4"))

        XCTAssertEqual(player.seekValues, [60])
        XCTAssertEqual(player.playCount, 1)
    }

    private func makeCoordinator(
        api: PlayerFakeAPI,
        player: PlayerFakeController,
        store: PlaybackProgressStore? = nil
    ) -> PlayerCoordinator {
        let progressStore: PlaybackProgressStore
        if let store {
            progressStore = store
        } else {
            let suite = "PlayerCoordinatorTests.\(UUID().uuidString)"
            let defaults = UserDefaults(suiteName: suite)!
            defaults.removePersistentDomain(forName: suite)
            progressStore = PlaybackProgressStore(defaults: defaults)
        }
        return PlayerCoordinator(
            api: api,
            controller: player,
            progressStore: progressStore,
            baseURL: URL(string: "https://alist.example")!,
            username: "alice"
        )
    }

    private func object(_ path: String) -> AListObject {
        AListObject(virtualPath: path, name: URL(fileURLWithPath: path).lastPathComponent, isDirectory: false)
    }

    private func detail(url: String, path: String) -> FileDetail {
        FileDetail(rawURL: url, virtualPath: path, name: "video.mp4", size: 100, isDirectory: false, modified: nil, created: nil, type: AListFileType.video.rawValue, thumbnail: nil)
    }

    private func waitUntil(condition: @escaping @MainActor () -> Bool) async throws {
        let clock = ContinuousClock()
        let deadline = clock.now + .seconds(2)
        while !condition() {
            if clock.now >= deadline {
                throw AListAPIError.transport(message: "Timed out waiting for player state")
            }
            try await Task.sleep(for: .milliseconds(10))
        }
    }
}

private actor PlayerFakeAPI: AListAPI {
    private var details: [FileDetail]
    private let log: LockedEventLog?
    private(set) var getPaths: [String] = []

    init(details: [FileDetail], log: LockedEventLog? = nil) {
        self.details = details
        self.log = log
    }

    func login(username: String, password: String, otpCode: String?) async throws -> LoginData { throw AListAPIError.invalidResponse }
    func currentUser() async throws -> CurrentUser { throw AListAPIError.invalidResponse }
    func list(path: String, page: Int, perPage: Int) async throws -> DirectoryPage { throw AListAPIError.invalidResponse }

    func get(path: String) async throws -> FileDetail {
        getPaths.append(path)
        log?.append("get:\(path)")
        guard !details.isEmpty else { throw AListAPIError.invalidResponse }
        return details.removeFirst()
    }
}

@MainActor
private final class PlayerFakeController: PlayerControlling {
    let events: AsyncStream<PlayerEvent>
    var currentTime: TimeInterval = 0
    var duration: TimeInterval = 120
    private(set) var replacedURLs: [URL] = []
    private(set) var seekValues: [TimeInterval] = []
    private(set) var playCount = 0
    private var continuation: AsyncStream<PlayerEvent>.Continuation!
    private let log: LockedEventLog?

    init(log: LockedEventLog? = nil) {
        self.log = log
        var captured: AsyncStream<PlayerEvent>.Continuation!
        events = AsyncStream { captured = $0 }
        continuation = captured
    }

    func replaceCurrentItem(url: URL) {
        replacedURLs.append(url)
        log?.append("replace:\(url.absoluteString)")
    }
    func play() { playCount += 1 }
    func pause() { continuation.yield(.paused) }
    func seek(to seconds: TimeInterval) async {
        seekValues.append(seconds)
        currentTime = seconds
    }
    func emit(_ event: PlayerEvent) { continuation.yield(event) }
}

private final class LockedEventLog: @unchecked Sendable {
    private let lock = NSLock()
    private var storage: [String] = []
    var values: [String] {
        lock.lock(); defer { lock.unlock() }
        return storage
    }
    func append(_ value: String) {
        lock.lock(); storage.append(value); lock.unlock()
    }
}
