import Combine
import Foundation
import UIKit

@MainActor
final class BrowserViewModel: ObservableObject {
    enum State: Equatable {
        case idle
        case loading
        case loaded
        case empty
        case loadingMore
        case failed(message: String)
        case forbidden
        case loadMoreFailed(message: String)
    }
    private enum ArtworkState {
        case loading
        case loaded
        case failed
    }


    @Published private(set) var state: State = .idle
    @Published private(set) var path = "/"
    @Published private(set) var items: [AListObject] = []
    @Published var focusedVirtualPath: String?
    @Published private(set) var hasMore = false
    @Published private(set) var artworkRevision = 0


    private struct PageKey: Hashable {
        let path: String
        let page: Int
    }

    private let api: any AListAPI
    private let perPage: Int
    private let onPlay: @MainActor (AListObject) async -> Void
    private let onUnauthorized: @MainActor () async -> Void
    private var directoryItems: [AListObject] = []
    private var fileItems: [AListObject] = []
    private var seenPaths: Set<String> = []
    private var requestedPages: Set<PageKey> = []
    private var inFlight: Set<PageKey> = []
    private var nextPage = 1
    private var generation = UUID()
    private var loadTask: Task<Void, Never>?
    private var focusHistory: [String: String] = [:]
    private var pendingRestoredFocus: String?
    private var failedNextPage: Int?

    private let artworkCache = NSCache<NSString, UIImage>()
    private var artworkTasks: [String: Task<Void, Never>] = [:]
    private var artworkStates: [String: ArtworkState] = [:]


    init(
        api: any AListAPI,
        perPage: Int = 200,
        onPlay: @escaping @MainActor (AListObject) async -> Void = { _ in },
        onUnauthorized: @escaping @MainActor () async -> Void = {}
    ) {
        self.api = api
        self.perPage = perPage
        self.onPlay = onPlay
        self.onUnauthorized = onUnauthorized
        artworkCache.totalCostLimit = 48 * 1024 * 1024
    }

    deinit {
        loadTask?.cancel()
        artworkTasks.values.forEach { $0.cancel() }
    }


    func loadInitial() {
        beginLoading(path: path, restoredFocus: pendingRestoredFocus)
    }

    func loadNextPageIfNeeded(currentItem: AListObject) {
        guard hasMore, !items.isEmpty,
              let currentPath = currentItem.virtualPath,
              let index = items.firstIndex(where: { $0.virtualPath == currentPath }) else {
            return
        }
        let threshold = max(0, items.count - min(8, items.count))
        guard index >= threshold else { return }
        requestPage(nextPage, initial: false, generation: generation, requestedPath: path)
    }

    func open(_ object: AListObject) {
        guard object.isDirectory else {
            Task { await onPlay(object) }
            return
        }
        do {
            let childPath = try AListPath.join(parent: path, name: object.name)
            if let focusedPath = object.virtualPath {
                focusHistory[path] = focusedPath
            }
            path = childPath
            pendingRestoredFocus = nil
            beginLoading(path: childPath, restoredFocus: nil)
        } catch {
            state = .failed(message: "This folder has an invalid name.")
        }
    }

    func moveToParent() {
        guard path != "/" else { return }
        let parent = AListPath.parent(of: path)
        path = parent
        let restored = focusHistory[parent]
        pendingRestoredFocus = restored
        beginLoading(path: parent, restoredFocus: restored)
    }
    func retry() {
        if let page = failedNextPage, !items.isEmpty {
            requestPage(page, initial: false, generation: generation, requestedPath: path)
        } else {
            beginLoading(path: path, restoredFocus: pendingRestoredFocus)
        }
    }

    func artwork(for object: AListObject) -> UIImage? {
        artworkCache.object(forKey: artworkKey(for: object))
    }

    func loadArtwork(for object: AListObject) {
        guard object.fileType == .audio || object.fileType == .video,
              let path = object.virtualPath else { return }
        let key = artworkKey(for: object) as String
        guard artworkStates[key] == nil else { return }

        artworkStates[key] = .loading
        artworkTasks[key] = Task { [weak self, api, onUnauthorized] in
            defer { self?.artworkTasks[key] = nil }
            do {
                let detail = try await api.get(path: path)
                let url = try PlayableURLValidator.validate(detail.rawURL)
                await ArtworkLoadLimiter.shared.acquire()
                let image = await MediaArtworkLoader.load(from: url, type: object.fileType)
                await ArtworkLoadLimiter.shared.release()

                guard let image, !Task.isCancelled else {
                    self?.artworkStates[key] = .failed
                    return
                }
                self?.artworkCache.setObject(image, forKey: key as NSString, cost: Self.artworkCost(image))
                self?.artworkRevision &+= 1
                self?.artworkStates[key] = .loaded
            } catch AListAPIError.unauthorized {
                self?.artworkStates[key] = .failed
                await onUnauthorized()
            } catch {
                self?.artworkStates[key] = .failed
            }
        }
    }

    private func artworkKey(for object: AListObject) -> NSString {
        "\(object.virtualPath ?? object.name)#\(object.modified ?? "")" as NSString
    }

    private static func artworkCost(_ image: UIImage) -> Int {
        guard let cgImage = image.cgImage else { return 0 }
        return cgImage.width * cgImage.height * 4
    }


    private func beginLoading(path requestedPath: String, restoredFocus: String?) {
        loadTask?.cancel()
        generation = UUID()
        let currentGeneration = generation
        inFlight.removeAll()
        requestedPages.removeAll()
        directoryItems.removeAll(keepingCapacity: true)
        fileItems.removeAll(keepingCapacity: true)
        seenPaths.removeAll(keepingCapacity: true)
        items.removeAll(keepingCapacity: true)
        hasMore = false
        nextPage = 1
        pendingRestoredFocus = restoredFocus
        failedNextPage = nil
        focusedVirtualPath = nil
        state = .loading
        loadTask = Task { [weak self] in
            await self?.fetchPage(1, initial: true, generation: currentGeneration, requestedPath: requestedPath, restoredFocus: restoredFocus)
        }
    }

    private func requestPage(_ page: Int, initial: Bool, generation: UUID, requestedPath: String) {
        let key = PageKey(path: requestedPath, page: page)
        guard !requestedPages.contains(key), !inFlight.contains(key) else { return }
        inFlight.insert(key)
        if !initial { state = .loadingMore }
        loadTask = Task { [weak self] in
            await self?.fetchPage(page, initial: initial, generation: generation, requestedPath: requestedPath, restoredFocus: nil)
        }
    }

    private func fetchPage(
        _ pageNumber: Int,
        initial: Bool,
        generation requestGeneration: UUID,
        requestedPath: String,
        restoredFocus: String?
    ) async {
        let key = PageKey(path: requestedPath, page: pageNumber)
        if !inFlight.contains(key) { inFlight.insert(key) }
        defer { inFlight.remove(key) }

        do {
            let page = try await api.list(path: requestedPath, page: pageNumber, perPage: perPage)
            guard !Task.isCancelled, generation == requestGeneration, path == requestedPath else { return }
            requestedPages.insert(key)
            appendPage(page.content, parentPath: requestedPath)
            hasMore = page.hasMore ?? (page.content.count == perPage)
            nextPage = pageNumber + 1
            failedNextPage = nil
            state = items.isEmpty && !hasMore ? .empty : .loaded
            let targetFocus = restoredFocus ?? pendingRestoredFocus
            if let targetFocus, items.contains(where: { $0.virtualPath == targetFocus }) {
                focusedVirtualPath = targetFocus
                pendingRestoredFocus = nil
            } else if targetFocus != nil, hasMore {
                requestPage(nextPage, initial: false, generation: requestGeneration, requestedPath: requestedPath)
            } else if !hasMore {
                pendingRestoredFocus = nil
            }
        } catch AListAPIError.unauthorized {
            guard generation == requestGeneration, path == requestedPath else { return }
            await onUnauthorized()
        } catch let AListAPIError.server(code, _) where code == 403 {
            guard generation == requestGeneration, path == requestedPath else { return }
            failedNextPage = initial ? nil : pageNumber
            state = .forbidden
        } catch {
            guard !Task.isCancelled, generation == requestGeneration, path == requestedPath else { return }
            failedNextPage = initial ? nil : pageNumber
            state = initial
                ? .failed(message: Self.message(for: error))
                : .loadMoreFailed(message: Self.message(for: error))
        }
    }

    private func appendPage(_ page: [AListObject], parentPath: String) {
        for object in page {
            let virtualPath: String
            if let provided = object.virtualPath, !provided.isEmpty {
                virtualPath = AListPath.normalize(provided)
            } else if let derived = try? AListPath.join(parent: parentPath, name: object.name) {
                virtualPath = derived
            } else {
                continue
            }
            guard seenPaths.insert(virtualPath).inserted else { continue }
            let normalized = object.withVirtualPath(virtualPath)
            if normalized.isDirectory {
                directoryItems.append(normalized)
            } else {
                fileItems.append(normalized)
            }
        }
        items = directoryItems + fileItems
    }

    private static func message(for error: Error) -> String {
        (error as? LocalizedError)?.errorDescription ?? error.localizedDescription
    }
}
