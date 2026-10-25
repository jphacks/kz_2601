import Foundation

/// video field の値(#206)。動画本体は撮影した端末のアプリ領域にだけ置き、レコードには
/// `"video:<id>|data:image/jpeg;base64,<1 フレーム目のサムネイル>"` だけを入れる(Firestore の 1 ドキュメント 1MB 制限)。
/// `https://` は将来の Firebase Storage 用(今は再生のみ対応)。
public struct VideoValue: Equatable, Sendable {
    /// 端末内のファイル id(`LocalVideoStore` のファイル名)。https の時は nil
    public var id: String?
    /// サムネイル(data URL)
    public var thumbnail: String?
    /// https の動画 URL
    public var remoteURL: URL?

    public init(id: String?, thumbnail: String?, remoteURL: URL? = nil) {
        self.id = id
        self.thumbnail = thumbnail
        self.remoteURL = remoteURL
    }

    public static let prefix = "video:"

    public init?(_ value: JSONValue) {
        guard let s = value.stringValue else { return nil }
        self.init(string: s)
    }

    public init?(string s: String) {
        if s.hasPrefix("https://"), let url = URL(string: s) {
            self.init(id: nil, thumbnail: nil, remoteURL: url)
            return
        }
        guard s.hasPrefix(Self.prefix) else { return nil }
        let rest = s.dropFirst(Self.prefix.count)
        let parts = rest.split(separator: "|", maxSplits: 1, omittingEmptySubsequences: false)
        let id = String(parts[0])
        guard LocalVideoStore.isValidID(id) else { return nil }
        let thumb = parts.count > 1 && parts[1].hasPrefix("data:image/") ? String(parts[1]) : nil
        self.init(id: id, thumbnail: thumb)
    }

    /// レコードに保存する文字列。
    public var encoded: String {
        if let remoteURL { return remoteURL.absoluteString }
        return Self.prefix + (id ?? "") + (thumbnail.map { "|" + $0 } ?? "")
    }

    /// レコードの値の中にある端末内動画の id(削除時に一緒にファイルを消す)。
    public static func localIDs(in data: [String: JSONValue]) -> Set<String> {
        Set(data.values.compactMap { VideoValue($0)?.id })
    }
}

/// 端末内の動画ファイル置き場(Application Support/metamon-videos/<id>.mp4)。
/// レコード削除・アプリ削除・アカウント削除(#180 / #119)の時に `RecordStore` / `RecordSyncEngine` から消す。
public struct LocalVideoStore: Sendable {
    public let directory: URL

    public init(directory: URL? = nil) {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        self.directory = directory ?? base.appendingPathComponent("metamon-videos", isDirectory: true)
    }

    public static let shared = LocalVideoStore()
    public static let fileExtension = "mp4"

    /// id は英数・-・_ だけ(パスを組み立てるので `..` や `/` を通さない)。
    public static func isValidID(_ id: String) -> Bool {
        !id.isEmpty && id.count <= 80 && id.allSatisfy { ($0.isASCII && ($0.isLetter || $0.isNumber)) || $0 == "-" || $0 == "_" }
    }

    public func url(for id: String) -> URL? {
        guard Self.isValidID(id) else { return nil }
        return directory.appendingPathComponent("\(id).\(Self.fileExtension)")
    }

    public func exists(_ id: String) -> Bool {
        url(for: id).map { FileManager.default.fileExists(atPath: $0.path) } ?? false
    }

    /// `source` のファイルを新しい id で取り込む(move。失敗したら copy)。
    public func importFile(at source: URL, id: String = UUID().uuidString) throws -> String {
        guard let dest = url(for: id) else { throw CocoaError(.fileWriteInvalidFileName) }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        try? FileManager.default.removeItem(at: dest)
        do { try FileManager.default.moveItem(at: source, to: dest) } catch { try FileManager.default.copyItem(at: source, to: dest) }
        // iCloud バックアップの対象外(端末内の一時的な記録で、容量が大きいため)
        var u = dest
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? u.setResourceValues(values)
        return id
    }

    public func remove(_ ids: some Sequence<String>) {
        for id in ids { if let u = url(for: id) { try? FileManager.default.removeItem(at: u) } }
    }

    public func removeAll() {
        try? FileManager.default.removeItem(at: directory)
    }
}
