import Foundation

enum MediaKind: String, Codable, CaseIterable, Identifiable {
    case movie, series, book, game
    var id: String { rawValue }
    var title: String {
        switch self {
        case .movie: "Filme"
        case .series: "Serien"
        case .book: "Bücher"
        case .game: "Spiele"
        }
    }
    var symbol: String {
        switch self {
        case .movie: "film"
        case .series: "tv"
        case .book: "book"
        case .game: "gamecontroller"
        }
    }
    var statuses: [String] {
        switch self {
        case .movie, .series: ["want", "started", "seen"]
        case .book: ["want", "started", "finished", "read"]
        case .game: ["library", "want", "started", "finished"]
        }
    }
    var criteria: [String] {
        switch self {
        case .book: ["Geschichte", "Figuren", "Schreibstil"]
        case .game: ["Qualität", "Gameplay", "Geschichte"]
        default: ["Qualität", "Geschichte", "Gesamteindruck"]
        }
    }
}

struct Media: Codable, Identifiable, Equatable {
    var id: String
    var revision: String?
    var kind: MediaKind
    var title = ""
    var subtitle: String?
    var status = "want"
    var progress = 0
    var total = 0
    var ratings = [0, 0, 0]
    var favorite: Bool?
    var notes: String?
    var genres: [String]?
    var lists: [String]?
    var cover: String?
    var customCover: String?

    var statusLabel: String {
        switch status {
        case "seen": "Gesehen"
        case "started": "Angefangen"
        case "finished": "Abgeschlossen"
        case "read": "Gelesen"
        case "library": "In der Sammlung"
        default: "Merkliste"
        }
    }

    static func blank(kind: MediaKind) -> Media {
        Media(id: "manual:" + UUID().uuidString.lowercased(), revision: nil, kind: kind)
    }

    func fieldsChanged(from base: Media) -> [String] {
        var fields: [String] = []
        if title != base.title { fields.append("title") }
        if (subtitle ?? "") != (base.subtitle ?? "") { fields.append("subtitle") }
        if status != base.status { fields.append("status") }
        if progress != base.progress { fields.append("progress") }
        if total != base.total { fields.append("total") }
        if ratings != base.ratings { fields.append("ratings") }
        if (favorite ?? false) != (base.favorite ?? false) { fields.append("favorite") }
        if (notes ?? "") != (base.notes ?? "") { fields.append("notes") }
        if (genres ?? []) != (base.genres ?? []) { fields.append("genres") }
        if (lists ?? []) != (base.lists ?? []) { fields.append("lists") }
        return fields
    }

    mutating func merge(_ new: Media, fields: [String]) {
        for field in fields {
            switch field {
            case "title": title = new.title
            case "subtitle": subtitle = new.subtitle
            case "status": status = new.status
            case "progress": progress = new.progress
            case "total": total = new.total
            case "ratings": ratings = new.ratings
            case "favorite": favorite = new.favorite
            case "notes": notes = new.notes
            case "genres": genres = new.genres
            case "lists": lists = new.lists
            default: break
            }
        }
    }
}

struct Change: Codable, Identifiable {
    let id: String
    let itemId: String
    let action: String
    let entry: Media?
    let base: Media?
    let fields: [String]?

    init(_ action: String, item: Media, base: Media? = nil, fields: [String]? = nil) {
        self.id = UUID().uuidString.lowercased()
        self.itemId = item.id
        self.action = action
        self.entry = action == "delete" ? nil : item
        self.base = base
        self.fields = fields
    }
}

struct MergeConflict: Codable, Identifiable {
    let id: String
    let current: Media?
}

struct SyncReply: Decodable {
    let entries: [Media]?
    let lists: [String]?
    let accepted: [String]
    let conflicts: [MergeConflict]
}

struct LibraryState: Codable {
    var entries: [Media] = []
    var lists: [String] = []
    var pending: [Change] = []
    var conflicts: [MergeConflict] = []

    var visible: [Media] {
        var items = Dictionary(uniqueKeysWithValues: entries.map { ($0.id, $0) })
        for change in pending {
            switch change.action {
            case "add":
                if let entry = change.entry { items[entry.id] = entry }
            case "edit":
                if let entry = change.entry {
                    var current = items[entry.id] ?? entry
                    current.merge(entry, fields: change.fields ?? [])
                    items[entry.id] = current
                }
            case "delete":
                items.removeValue(forKey: change.itemId)
            default: break
            }
        }
        return items.values.sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
    }
}
