import Foundation

/// `fileImport.map` の 1 つ: CSV の 1 行目が `column` の列を `field` に入れる。
public struct ImportColumn: Decodable, Equatable, Sendable {
    public var column: String
    public var field: String

    public init(column: String, field: String) {
        self.column = column
        self.field = field
    }
}

/// RFC 4180 の CSV を読む(`CSVWriter` の逆)。引用符・`""`・セル内の改行・CRLF / LF / CR・BOM に対応。
public enum CSVReader {
    /// バイト列を文字にする。UTF-8(BOM の有無どちらも)→ だめなら Shift_JIS(日本語の Excel の「CSV」)。
    public static func decode(_ data: Data) -> String? {
        var bytes = data
        if bytes.starts(with: CSVWriter.bom) { bytes = bytes.dropFirst(3) }
        if let s = String(data: bytes, encoding: .utf8) { return s }
        if data.starts(with: [0xFF, 0xFE]) || data.starts(with: [0xFE, 0xFF]) { return String(data: data, encoding: .utf16) }
        return String(data: data, encoding: .shiftJIS)
    }

    /// 行 × セル。空行(セルが 1 つで空)は飛ばす。
    public static func parse(_ text: String) -> [[String]] {
        var rows: [[String]] = []
        var row: [String] = []
        var cell = ""
        var quoted = false
        var scalars = text.unicodeScalars.makeIterator()
        var pending: Unicode.Scalar? = scalars.next()
        if pending == "\u{FEFF}" { pending = scalars.next() }
        func endRow() {
            row.append(cell)
            cell = ""
            if !(row.count == 1 && row[0].isEmpty) { rows.append(row) }
            row = []
        }
        while let c = pending {
            pending = scalars.next()
            if quoted {
                if c == "\"" {
                    if pending == "\"" {
                        cell.unicodeScalars.append("\"")
                        pending = scalars.next()
                    } else {
                        quoted = false
                    }
                } else {
                    cell.unicodeScalars.append(c)
                }
                continue
            }
            switch c {
            case "\"" where cell.isEmpty:
                quoted = true
            case ",":
                row.append(cell)
                cell = ""
            case "\r":
                if pending == "\n" { pending = scalars.next() }
                endRow()
            case "\n":
                endRow()
            default:
                cell.unicodeScalars.append(c)
            }
        }
        if !cell.isEmpty || !row.isEmpty { endRow() }
        return rows
    }
}

/// 読めなかった 1 行(行番号は表計算と同じ: 見出しが 1 行目)。
public struct ImportRowFailure: Equatable, Sendable {
    public enum Kind: Equatable, Sendable {
        /// 型に合わない(数にできない・選択肢に無い・参照先が無い など)
        case invalid
        /// 必須の項目が空
        case required
        /// 長すぎる(string 4000 字・text 20000 字)
        case tooLong
    }
    public var line: Int
    public var fieldName: String
    public var kind: Kind
    public var cell: String
}

/// CSV を記録に変える計画。確認ダイアログに件数を出し、OK なら `rows` を 1 件ずつ足す。
public struct ImportPlan: Equatable, Sendable {
    /// 当てた列(見出し → field 名)
    public var matched: [(column: String, field: String)]
    /// どの field にも当たらない列の見出し
    public var unused: [String]
    /// 足す記録(既定値を埋めた後)
    public var rows: [[String: JSONValue]]
    public var failures: [ImportRowFailure]
    /// 上限(1 回 500 件・entity の合計)で入れない行の数
    public var overLimit: Int

    public static func == (a: Self, b: Self) -> Bool {
        a.matched.map { $0.column } == b.matched.map { $0.column } && a.matched.map { $0.field } == b.matched.map { $0.field }
            && a.unused == b.unused && a.rows == b.rows && a.failures == b.failures && a.overLimit == b.overLimit
    }
}

/// `fileImport` の端末内の処理(ファイルは送らない・残さない)。UI と PDF の読み取りは Renderer。
public enum RecordImport {
    /// 1 回で足す最大の件数
    public static let maxRows = 500
    /// 1 entity の記録の上限(backend `store.MaxRecordsPerEntity`)
    public static let maxRecordsPerEntity = 2000
    /// 読むファイルの大きさの上限(CSV・テキスト / PDF)
    public static let maxTextBytes = 5_000_000
    public static let maxPDFBytes = 30_000_000
    /// 文字認識する PDF のページ数の上限(本文の無い、画像だけのページ)
    public static let maxOCRPages = 10

    /// field に入る最大の文字数(appconfig `recorddata.go` と同じ。数え方は Go の rune = Unicode scalar)
    public static func characterLimit(_ f: Field) -> Int { f.type == .text ? 20000 : 4000 }

    public enum CellValue: Equatable, Sendable {
        case empty
        case value(JSONValue)
        case invalid(ImportRowFailure.Kind)
    }

    // MARK: - 列の当て方

    /// 見出し → 使える field。`map` が先、残りは見出しが field の name か id と同じ列(前後の空白・大文字小文字は無視)。
    /// 計算項目・写真・動画・録音・参加者(member・uid は CSV に書けない)には当てない。1 つの field に当てるのは最初の列だけ。
    public static func columns(headers: [String], entity: Entity, map: [ImportColumn]) -> [Int: Field] {
        let usable = entity.storedFields.filter { ![.photo, .video, .audio, .member].contains($0.type) }
        let key = { (s: String) in s.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() }
        var out: [Int: Field] = [:]
        var used = Set<String>()
        for m in map {
            guard let f = usable.first(where: { $0.id == m.field }), !used.contains(f.id),
                  let i = headers.indices.first(where: { out[$0] == nil && key(headers[$0]) == key(m.column) }) else { continue }
            out[i] = f
            used.insert(f.id)
        }
        for (i, h) in headers.enumerated() where out[i] == nil {
            guard let f = usable.first(where: { !used.contains($0.id) && (key($0.name) == key(h) || key($0.id) == key(h)) }) else { continue }
            out[i] = f
            used.insert(f.id)
        }
        return out
    }

    // MARK: - 計画

    /// CSV の表から足す記録を作る。`refId` は ref / refs の名前 → 参照先レコードの id。
    /// `existing` は今の件数(合計 2000 件まで)。
    public static func plan(table: [[String]], entity: Entity, map: [ImportColumn], existing: Int,
                            now: Date = DSLClock.now(), refId: (Field, String) -> String?) -> ImportPlan {
        guard let headers = table.first else { return ImportPlan(matched: [], unused: [], rows: [], failures: [], overLimit: 0) }
        let cols = columns(headers: headers, entity: entity, map: map)
        let matched = cols.keys.sorted().map { (column: headers[$0], field: cols[$0]!.name) }
        let unused = headers.indices.filter { cols[$0] == nil }.map { headers[$0] }.filter { !$0.isEmpty }
        let room = max(0, min(maxRows, maxRecordsPerEntity - existing))
        var rows: [[String: JSONValue]] = []
        var failures: [ImportRowFailure] = []
        var over = 0
        guard !cols.isEmpty else { return ImportPlan(matched: [], unused: unused, rows: [], failures: [], overLimit: 0) }
        for (n, cells) in table.dropFirst().enumerated() {
            let line = n + 2
            var data: [String: JSONValue] = [:]
            var failure: ImportRowFailure?
            for (i, f) in cols.sorted(by: { $0.key < $1.key }) {
                let cell = i < cells.count ? cells[i] : ""
                switch value(cell, field: f, now: now, refId: refId) {
                case .empty: continue
                case .value(let v): data[f.id] = v
                case .invalid(let kind):
                    failure = failure ?? ImportRowFailure(line: line, fieldName: f.name, kind: kind, cell: String(cell.prefix(40)))
                }
            }
            if failure == nil {
                for f in entity.storedFields where data[f.id] == nil {
                    let d = FieldCoding.defaultValue(for: f)
                    if !d.isNull { data[f.id] = d }
                }
                if let f = entity.storedFields.first(where: { $0.required == true && (data[$0.id]?.isNull ?? true) }) {
                    failure = ImportRowFailure(line: line, fieldName: f.name, kind: .required, cell: "")
                }
            }
            if let failure {
                failures.append(failure)
            } else if rows.count < room {
                rows.append(data)
            } else {
                over += 1
            }
        }
        return ImportPlan(matched: matched, unused: unused, rows: rows, failures: failures, overLimit: over)
    }

    // MARK: - 型の変換

    /// 1 セル → field の値。`export` が書いた表示の書式(¥1,200・はい / いいえ・10/24(金)・参照先の名前)も読む。
    public static func value(_ raw: String, field f: Field, now: Date = DSLClock.now(), refId: (Field, String) -> String?) -> CellValue {
        var s = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        // export の CSV インジェクション対策(先頭の ')を外す
        if s.hasPrefix("'"), let next = s.unicodeScalars.dropFirst().first, "=+-@\t\r".unicodeScalars.contains(next) { s.removeFirst() }
        if s.isEmpty || s == FieldCoding.unavailable { return .empty }
        switch f.type {
        case .string, .text:
            // 文字はセルのまま(前後の空白も残す)
            var text = raw
            if text.hasPrefix("'"), let next = text.unicodeScalars.dropFirst().first, "=+-@\t\r".unicodeScalars.contains(next) { text.removeFirst() }
            return text.unicodeScalars.count > characterLimit(f) ? .invalid(.tooLong) : .value(.string(text))
        case .integer, .currency:
            guard let n = number(s, unit: f.unit) else { return .invalid(.invalid) }
            return .value(.number(n.rounded()))
        case .number:
            guard let n = number(s, unit: f.unit) else { return .invalid(.invalid) }
            return .value(.number(n))
        case .boolean:
            switch s.lowercased() {
            case "true", "yes", "y", "1", "on", "はい", "✓", "✔", "○", "〇", "済": return .value(.bool(true))
            case "false", "no", "n", "0", "off", "いいえ", "×", "✗", "未": return .value(.bool(false))
            default: return .invalid(.invalid)
            }
        case .date, .datetime:
            guard let d = date(s, now: now) else { return .invalid(.invalid) }
            return .value(.string(FieldCoding.encode(d, for: f)))
        case .enum:
            let options = f.options ?? []
            if options.contains(s) { return .value(.string(s)) }
            if let o = options.first(where: { $0.lowercased() == s.lowercased() }) { return .value(.string(o)) }
            return .invalid(.invalid)
        case .ref:
            return refId(f, s).map { .value(.string($0)) } ?? .invalid(.invalid)
        case .refs:
            if s == "全員" || s.lowercased() == "everyone" { return .empty }
            let names = s.replacingOccurrences(of: ", and ", with: ",").replacingOccurrences(of: " and ", with: ",")
                .split(whereSeparator: { "、,，".contains($0) })
                .map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
            var ids: [JSONValue] = []
            for name in names {
                guard let id = refId(f, name) else { return .invalid(.invalid) }
                ids.append(.string(id))
            }
            return ids.isEmpty ? .empty : .value(.array(ids))
        case .location:
            let parts = s.split(separator: ",").map { Double($0.trimmingCharacters(in: .whitespaces)) }
            guard parts.count == 2, let lat = parts[0], let lng = parts[1], abs(lat) <= 90, abs(lng) <= 180 else { return .invalid(.invalid) }
            return .value(.string(s.replacingOccurrences(of: " ", with: "")))
        case .photo, .video, .audio, .member:
            return .empty
        }
    }

    /// 「¥1,200」「1,200 円」「-¥500」「5 km」「１２」→ 数。単位・通貨記号・桁区切りを外す。
    static func number(_ s: String, unit: String?) -> Double? {
        var t = s.applyingTransform(.fullwidthToHalfwidth, reverse: false) ?? s
        if let unit, !unit.isEmpty, t.hasSuffix(unit) { t.removeLast(unit.count) }
        if let suffix = FieldCoding.currencySuffix(unit: unit), t.hasSuffix(suffix) { t.removeLast(suffix.count) }
        for token in ["¥", "円", "$", ",", " "] { t = t.replacingOccurrences(of: token, with: "") }
        guard !t.isEmpty, let n = Double(t), n.isFinite else { return nil }
        return n
    }

    private static let weekdayPattern = try! NSRegularExpression(pattern: #"[(（][^)）]*[)）]"#)

    /// 日付・日時を読む。RFC 3339・YYYY-MM-DD、export の「2026/10/24(金) 14:30」「10/24(金)」(年が無ければ `now` の年)、
    /// 「2026年10月24日」、英語 UI の「Fri, Oct 24, 2:30 PM」。
    public static func date(_ raw: String, now: Date = DSLClock.now()) -> Date? {
        if let d = FieldCoding.date(from: .string(raw)) { return d }
        let ns = raw as NSString
        let s = weekdayPattern.stringByReplacingMatches(in: raw, range: NSRange(location: 0, length: ns.length), withTemplate: "")
            .replacingOccurrences(of: "  ", with: " ").trimmingCharacters(in: .whitespaces)
        let formats = ["yyyy/M/d H:mm:ss", "yyyy/M/d H:mm", "yyyy/M/d", "yyyy-M-d H:mm:ss", "yyyy-M-d H:mm", "yyyy-M-d'T'H:mm",
                       "yyyy年M月d日 H:mm", "yyyy年M月d日", "M/d H:mm", "M/d", "M月d日 H:mm", "M月d日",
                       "EEE, MMM d, yyyy, h:mm a", "EEE, MMM d, h:mm a", "EEE, MMM d, yyyy", "EEE, MMM d", "MMM d, yyyy", "MMM d"]
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.calendar = Calendar(identifier: .gregorian)
        f.timeZone = FieldCoding.dateFormatter.timeZone
        f.isLenient = false
        // 年の無い書式は今年(export は今年の日付の年を省く)
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = f.timeZone
        f.defaultDate = cal.date(from: DateComponents(year: cal.component(.year, from: now), month: 1, day: 1))
        for format in formats {
            f.dateFormat = format
            if let d = f.date(from: s) { return d }
        }
        return nil
    }

    // MARK: - テキスト / PDF

    /// テキストファイルの中身(UTF-8 / UTF-16 / Shift_JIS)。
    public static func plainText(_ data: Data) -> String? {
        CSVReader.decode(data)
    }

    /// 本文を 1 件の記録にする。本文は `field` の上限で切り(切ったら `truncated`)、entity の titleField(string / text)にはファイル名(拡張子なし)。
    public static func bodyRecord(text: String, fileName: String, entity: Entity, field: String) -> (data: [String: JSONValue], truncated: Bool)? {
        guard let f = entity.storedFields.first(where: { $0.id == field && ($0.type == .text || $0.type == .string) }) else { return nil }
        let body = text.replacingOccurrences(of: "\r\n", with: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
        let limit = characterLimit(f)
        var data: [String: JSONValue] = [:]
        let truncated = body.unicodeScalars.count > limit
        data[f.id] = .string(truncated ? String(String.UnicodeScalarView(body.unicodeScalars.prefix(limit))) : body)
        if let tid = entity.titleField, tid != f.id, let t = entity.storedFields.first(where: { $0.id == tid }), t.type == .string || t.type == .text {
            let name = (fileName as NSString).deletingPathExtension.trimmingCharacters(in: .whitespaces)
            if !name.isEmpty { data[t.id] = .string(String(name.prefix(200))) }
        }
        for x in entity.storedFields where data[x.id] == nil {
            let d = FieldCoding.defaultValue(for: x)
            if !d.isNull { data[x.id] = d }
        }
        return (data, truncated)
    }
}
