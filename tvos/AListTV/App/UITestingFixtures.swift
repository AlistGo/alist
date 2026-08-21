#if DEBUG
import Foundation

final class InMemoryCredentialStore: CredentialStore, @unchecked Sendable {
    private let lock = NSLock()
    private var token: String?

    func loadToken() throws -> String? {
        lock.lock()
        defer { lock.unlock() }
        return token
    }

    func saveToken(_ token: String) throws {
        lock.lock()
        self.token = token
        lock.unlock()
    }

    func deleteToken() throws {
        lock.lock()
        token = nil
        lock.unlock()
    }
}

actor FixtureAListAPI: AListAPI {
    func login(username: String, password: String, otpCode: String?) async throws -> LoginData {
        if username == "otp", otpCode != "123456" {
            throw AListAPIError.otpRequired
        }
        return LoginData(token: "fixture-token", deviceKey: "fixture-device")
    }

    func currentUser() async throws -> CurrentUser {
        CurrentUser(id: 1, username: "fixture")
    }

    func list(path: String, page: Int, perPage: Int) async throws -> DirectoryPage {
        let content: [AListObject]
        if path == "/" {
            content = [
                AListObject(virtualPath: "/Shows", name: "Shows", isDirectory: true, type: 1),
                AListObject(virtualPath: "/Sample.mp4", name: "Sample.mp4", size: 12_000_000, isDirectory: false, type: AListFileType.video.rawValue)
            ]
        } else {
            content = [
                AListObject(virtualPath: "\(path)/Episode.mp4", name: "Episode.mp4", size: 24_000_000, isDirectory: false, type: AListFileType.video.rawValue)
            ]
        }
        return DirectoryPage(content: content, hasMore: false, page: page, perPage: perPage)
    }

    func get(path: String) async throws -> FileDetail {
        FileDetail(
            rawURL: "https://media.example.test/video.mp4",
            virtualPath: path,
            name: URL(fileURLWithPath: path).lastPathComponent,
            size: 12_000_000,
            isDirectory: false,
            modified: nil,
            created: nil,
            type: AListFileType.video.rawValue,
            thumbnail: nil
        )
    }
}

@MainActor
final class FixturePlayerController: PlayerControlling {
    let events: AsyncStream<PlayerEvent>
    private var continuation: AsyncStream<PlayerEvent>.Continuation!
    private(set) var currentTime: TimeInterval = 45
    private(set) var duration: TimeInterval = 120

    init() {
        var captured: AsyncStream<PlayerEvent>.Continuation!
        events = AsyncStream { captured = $0 }
        continuation = captured
    }

    func replaceCurrentItem(url: URL) {}
    func play() {}
    func pause() { continuation.yield(.paused) }
    func seek(to seconds: TimeInterval) async { currentTime = seconds }

    deinit { continuation.finish() }
}
#endif
