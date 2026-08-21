import Combine
import Foundation

enum PlayableURLValidator {
    static func validate(_ rawValue: String) throws -> URL {
        let value = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !value.isEmpty,
              let components = URLComponents(string: value),
              components.scheme?.lowercased() == "https",
              let host = components.host, !host.isEmpty,
              components.user == nil,
              components.password == nil,
              let url = components.url else {
            throw AListAPIError.invalidRawURL
        }
        return url
    }
}

@MainActor
final class PlayerCoordinator: ObservableObject {
    enum State: Equatable {
        case idle
        case loading
        case playing
        case failed(message: String)
    }

    @Published private(set) var state: State = .idle
    @Published var isPresented = false

    let controller: any PlayerControlling

    private let api: any AListAPI
    private let progressStore: PlaybackProgressStore
    private let baseURL: URL
    private let username: String
    private let onUnauthorized: @MainActor () async -> Void
    private var currentObject: AListObject?
    private var sessionID = UUID()
    private var refreshCount = 0
    private var eventTask: Task<Void, Never>?
    private var progressTask: Task<Void, Never>?

    init(
        api: any AListAPI,
        controller: any PlayerControlling,
        progressStore: PlaybackProgressStore,
        baseURL: URL,
        username: String,
        onUnauthorized: @escaping @MainActor () async -> Void = {}
    ) {
        self.api = api
        self.controller = controller
        self.progressStore = progressStore
        self.baseURL = baseURL
        self.username = username
        self.onUnauthorized = onUnauthorized
    }

    deinit {
        eventTask?.cancel()
        progressTask?.cancel()
    }

    func play(object: AListObject) async {
        guard !object.isDirectory, let path = object.virtualPath, !path.isEmpty else {
            state = .failed(message: "Folders cannot be played.")
            return
        }

        finishMonitoring(saveProgress: true)
        currentObject = object
        sessionID = UUID()
        let activeSession = sessionID
        refreshCount = 0
        state = .loading

        do {
            let detail = try await api.get(path: path)
            guard sessionID == activeSession else { return }
            let url = try PlayableURLValidator.validate(detail.rawURL)
            controller.replaceCurrentItem(url: url)
            await restoreProgress()
            guard sessionID == activeSession else { return }
            controller.play()
            state = .playing
            isPresented = true
            startMonitoring(session: activeSession)
        } catch AListAPIError.unauthorized {
            await onUnauthorized()
        } catch {
            isPresented = true
            state = .failed(message: Self.message(for: error))
        }
    }

    func playerDidDisappear() {
        finishMonitoring(saveProgress: true)
        controller.pause()
        isPresented = false
        state = .idle
        currentObject = nil
    }

    func saveProgress() {
        guard let identity = progressIdentity else { return }
        progressStore.update(
            identity: identity,
            position: controller.currentTime,
            duration: controller.duration
        )
    }

    private func startMonitoring(session: UUID) {
        if eventTask == nil {
            let events = controller.events
            eventTask = Task { [weak self] in
                for await event in events {
                    guard !Task.isCancelled, let self else { return }
                    guard currentObject != nil else { continue }
                    let eventSession = sessionID
                    switch event {
                    case let .failed(message):
                        await handleFailure(message, session: eventSession)
                    case .paused:
                        saveProgress()
                    }
                }
            }
        }
        progressTask?.cancel()
        progressTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(10))
                guard !Task.isCancelled, let self, self.sessionID == session else { return }
                self.saveProgress()
            }
        }
    }

    private func handleFailure(_ message: String, session: UUID) async {
        guard sessionID == session,
              let path = currentObject?.virtualPath else { return }
        guard refreshCount == 0 else {
            controller.pause()
            state = .failed(message: message)
            return
        }

        refreshCount = 1
        let position = controller.currentTime
        do {
            let detail = try await api.get(path: path)
            guard sessionID == session else { return }
            let url = try PlayableURLValidator.validate(detail.rawURL)
            controller.replaceCurrentItem(url: url)
            if position > 0 {
                await controller.seek(to: position)
            }
            guard sessionID == session else { return }
            controller.play()
            state = .playing
        } catch AListAPIError.unauthorized {
            await onUnauthorized()
        } catch {
            controller.pause()
            state = .failed(message: Self.message(for: error))
        }
    }

    private func restoreProgress() async {
        guard let identity = progressIdentity,
              let record = progressStore.record(for: identity),
              record.position >= 30,
              record.duration > 0,
              record.position / record.duration < 0.9 else {
            return
        }
        await controller.seek(to: record.position)
    }

    private func finishMonitoring(saveProgress shouldSave: Bool) {
        if shouldSave { saveProgress() }
        progressTask?.cancel()
        progressTask = nil
    }

    private var progressIdentity: PlaybackProgressIdentity? {
        guard let path = currentObject?.virtualPath else { return nil }
        return PlaybackProgressIdentity(
            baseURL: baseURL.absoluteString,
            username: username,
            virtualPath: path
        )
    }

    private static func message(for error: Error) -> String {
        (error as? LocalizedError)?.errorDescription ?? error.localizedDescription
    }
}
