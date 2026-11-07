import Foundation

/// `lookupBook` の中身。フォームの entity の field id(Go の `dsl.BookLookup` と 1:1)。
public struct BookLookupSpec: Decodable, Equatable, Sendable {
    public var isbnField: String
    public var titleField: String?
    public var authorsField: String?
    public var publisherField: String?
    public var publishedDateField: String?

    public init(isbnField: String, titleField: String? = nil, authorsField: String? = nil,
                publisherField: String? = nil, publishedDateField: String? = nil) {
        self.isbnField = isbnField
        self.titleField = titleField
        self.authorsField = authorsField
        self.publisherField = publisherField
        self.publishedDateField = publishedDateField
    }
}

/// `GET /v1/lookup/book` の応答。
public struct BookInfo: Decodable, Equatable, Sendable {
    public struct Source: Decodable, Equatable, Sendable {
        public var name: String
        public var url: String
        public var license: String?
    }

    public var isbn: String
    public var title: String
    public var authors: String?
    public var publisher: String?
    /// YYYY-MM-DD / YYYY-MM / YYYY
    public var publishedDate: String?
    public var coverUrl: String?
    public var source: Source

    public init(isbn: String, title: String, authors: String? = nil, publisher: String? = nil,
                publishedDate: String? = nil, coverUrl: String? = nil, source: Source) {
        self.isbn = isbn
        self.title = title
        self.authors = authors
        self.publisher = publisher
        self.publishedDate = publishedDate
        self.coverUrl = coverUrl
        self.source = source
    }
}

extension BookInfo.Source {
    public init(name: String, url: String) { self.init(name: name, url: url, license: nil) }
}

/// ISBN の確かめ(サーバーの `books.NormalizeISBN` と同じ規則)と、見つかった本 → フォームの値。
public enum BookLookupPlan {
    /// ISBN-10 / ISBN-13(ハイフン・空白は可)を ISBN-13 の数字に。検査数字が合わない・978 / 979 で始まらない時は nil。
    public static func normalizeISBN(_ raw: String) -> String? {
        guard raw.count <= 40 else { return nil }
        var s = ""
        for ch in raw {
            switch ch {
            case "0"..."9": s.append(ch)
            case "x", "X": s.append("X")
            case "-", " ", "\u{3000}": continue
            default: return nil
            }
        }
        let d = Array(s)
        switch d.count {
        case 10:
            var sum = 0
            for (i, c) in d.enumerated() {
                let v: Int
                if c == "X" {
                    guard i == 9 else { return nil }
                    v = 10
                } else {
                    v = c.wholeNumberValue ?? 0
                }
                sum += v * (10 - i)
            }
            guard sum % 11 == 0 else { return nil }
            return isbn13("978" + String(d[0..<9]))
        case 13:
            guard !s.contains("X"), s.hasPrefix("978") || s.hasPrefix("979") else { return nil }
            return isbn13(String(d[0..<12])) == s ? s : nil
        default:
            return nil
        }
    }

    private static func isbn13(_ twelve: String) -> String {
        var sum = 0
        for (i, c) in twelve.enumerated() {
            let v = c.wholeNumberValue ?? 0
            sum += i % 2 == 1 ? v * 3 : v
        }
        return twelve + String((10 - sum % 10) % 10)
    }

    /// 埋める値(field id → 値)。entity に無い field・型の合わない field・空の値は入れない。
    /// 出版日は date の field なら「2014-04」→ 2014-04-01 のように日付にする。
    public static func values(_ spec: BookLookupSpec, book: BookInfo, entity: Entity) -> [String: JSONValue] {
        var out: [String: JSONValue] = [:]
        func put(_ id: String?, _ value: String?, allowDate: Bool = false) {
            guard let id, let f = entity.field(id), f.computed == nil,
                  let v = value?.trimmingCharacters(in: .whitespacesAndNewlines), !v.isEmpty else { return }
            switch f.type {
            case .string, .text:
                out[id] = .string(v)
            case .date where allowDate:
                if let d = dateString(v) { out[id] = .string(d) }
            default:
                break
            }
        }
        put(spec.titleField, book.title)
        put(spec.authorsField, book.authors)
        put(spec.publisherField, book.publisher)
        put(spec.publishedDateField, book.publishedDate, allowDate: true)
        return out
    }

    /// YYYY / YYYY-MM / YYYY-MM-DD → YYYY-MM-DD(足りない所は 01)。
    static func dateString(_ s: String) -> String? {
        let parts = s.split(separator: "-").map(String.init)
        guard (1...3).contains(parts.count), parts[0].count == 4, parts.allSatisfy({ $0.allSatisfy(\.isNumber) }) else { return nil }
        let padded = parts + Array(repeating: "01", count: 3 - parts.count)
        let out = padded.joined(separator: "-")
        return FieldCoding.dateFormatter.date(from: out) == nil ? nil : out
    }
}
