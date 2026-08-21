import Foundation

struct PlaybackProgressIdentity: Codable, Hashable, Sendable {
    let baseURL: String
    let username: String
    let virtualPath: String
}

struct PlaybackProgressRecord: Codable, Equatable, Sendable {
    let identity: PlaybackProgressIdentity
    var position: TimeInterval
    var duration: TimeInterval
    var updatedAt: Date
}

struct PlaybackProgressStore {
    static let defaultsKey = "com.alist.tv.playback-progress-v1"
    private let defaults: UserDefaults
    private let now: () -> Date
    private let encoder = PropertyListEncoder()
    private let decoder = PropertyListDecoder()

    init(defaults: UserDefaults = .standard, now: @escaping () -> Date = { Date() }) {
        self.defaults = defaults
        self.now = now
    }

    func record(for identity: PlaybackProgressIdentity) -> PlaybackProgressRecord? {
        records().first { $0.identity == identity }
    }

    func update(identity: PlaybackProgressIdentity, position: TimeInterval, duration: TimeInterval) {
        guard position.isFinite, duration.isFinite, duration > 0 else { return }
        var all = records()
        let ratio = max(0, position) / duration
        if ratio >= 0.9 {
            all.removeAll { $0.identity == identity }
            save(all)
            return
        }
        guard position >= 30 else { return }

        let record = PlaybackProgressRecord(
            identity: identity,
            position: min(position, duration),
            duration: duration,
            updatedAt: now()
        )
        if let index = all.firstIndex(where: { $0.identity == identity }) {
            all[index] = record
        } else {
            all.append(record)
        }
        all.sort { $0.updatedAt > $1.updatedAt }
        if all.count > 500 {
            all.removeSubrange(500...)
        }
        save(all)
    }

    func remove(identity: PlaybackProgressIdentity) {
        var all = records()
        all.removeAll { $0.identity == identity }
        save(all)
    }

    func allRecords() -> [PlaybackProgressRecord] {
        records()
    }

    private func records() -> [PlaybackProgressRecord] {
        guard let data = defaults.data(forKey: Self.defaultsKey),
              let decoded = try? decoder.decode([PlaybackProgressRecord].self, from: data) else {
            return []
        }
        return decoded
    }

    private func save(_ records: [PlaybackProgressRecord]) {
        guard let data = try? encoder.encode(records) else { return }
        defaults.set(data, forKey: Self.defaultsKey)
    }
}
