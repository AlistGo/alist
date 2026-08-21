import Foundation

enum AListPathError: Error, Equatable {
    case invalidName
}

enum AListPath {
    static func join(parent: String, name: String) throws -> String {
        guard !name.isEmpty, !name.contains("/") else {
            throw AListPathError.invalidName
        }
        var components = normalizedComponents(parent)
        switch name {
        case ".":
            break
        case "..":
            if !components.isEmpty { components.removeLast() }
        default:
            components.append(name)
        }
        return components.isEmpty ? "/" : "/" + components.joined(separator: "/")
    }

    static func parent(of path: String) -> String {
        var components = normalizedComponents(path)
        if !components.isEmpty { components.removeLast() }
        return components.isEmpty ? "/" : "/" + components.joined(separator: "/")
    }

    static func normalize(_ path: String) -> String {
        let components = normalizedComponents(path)
        return components.isEmpty ? "/" : "/" + components.joined(separator: "/")
    }

    private static func normalizedComponents(_ path: String) -> [String] {
        var result: [String] = []
        for component in path.split(separator: "/", omittingEmptySubsequences: true).map(String.init) {
            switch component {
            case ".":
                continue
            case "..":
                if !result.isEmpty { result.removeLast() }
            default:
                result.append(component)
            }
        }
        return result
    }
}
