import Foundation

/// 共有シート(Share Extension)で受け取った 1 件(#2224)。
struct ShareInboxItem: Codable, Equatable, Identifiable {
    enum Kind: String, Codable {
        case text, url, image, pdf
    }

    var id: String
    var kind: Kind
    /// 文字・URL の中身。画像・PDF は nil
    var text: String?
    /// 置き場の中の添付ファイル名。文字・URL は nil
    var fileName: String?
    var receivedAt: Date
}

/// App Group に置く受け取り箱。拡張(MetamonShare)が書き、本体が `openURL` で開いた時に読む。
/// どの生成アプリに入れるかの振り分けは #1775 の決定待ちなので、ここは置く・読む・消すだけ。
struct ShareInbox {
    static let appGroupID = "group.jp.kz2601.metamon"
    static let directoryName = "ShareInbox"
    /// 受け取った後に本体を開くリンク。`ShareURL.token` は nil を返すのでシェアリンク(#49)とぶつからない。
    /// 読み取り側は Shared/Runtime/IncomingLink.swift の `.shareInbox` と揃える
    static let openURL = URL(string: "tailorit://share-inbox")!

    let directory: URL

    /// App Group の中の置き場。entitlement が無い時は nil
    static var shared: ShareInbox? {
        guard let container = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroupID) else { return nil }
        return ShareInbox(directory: container.appendingPathComponent(directoryName, isDirectory: true))
    }

    static func isOpenURL(_ url: URL) -> Bool {
        url.scheme?.lowercased() == openURL.scheme && url.host?.lowercased() == openURL.host
    }

    @discardableResult
    func put(kind: ShareInboxItem.Kind, text: String? = nil, data: Data? = nil, fileExtension: String? = nil, now: Date = .now) throws -> ShareInboxItem {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let id = UUID().uuidString
        var fileName: String?
        if let data {
            let name = fileExtension.map { "\(id).\($0)" } ?? id
            try data.write(to: directory.appendingPathComponent(name), options: .atomic)
            fileName = name
        }
        let item = ShareInboxItem(id: id, kind: kind, text: text, fileName: fileName, receivedAt: now)
        try JSONEncoder().encode(item).write(to: metaURL(id: id), options: .atomic)
        return item
    }

    /// 新しい順。読めない物は飛ばす
    func items() -> [ShareInboxItem] {
        let names = (try? FileManager.default.contentsOfDirectory(atPath: directory.path)) ?? []
        return names
            .filter { $0.hasSuffix(".json") }
            .compactMap { name -> ShareInboxItem? in
                guard let data = try? Data(contentsOf: directory.appendingPathComponent(name)) else { return nil }
                return try? JSONDecoder().decode(ShareInboxItem.self, from: data)
            }
            .sorted { $0.receivedAt > $1.receivedAt }
    }

    func fileURL(of item: ShareInboxItem) -> URL? {
        item.fileName.map { directory.appendingPathComponent($0) }
    }

    func remove(_ item: ShareInboxItem) {
        if let file = fileURL(of: item) { try? FileManager.default.removeItem(at: file) }
        try? FileManager.default.removeItem(at: metaURL(id: item.id))
    }

    func removeAll() {
        for item in items() { remove(item) }
    }

    private func metaURL(id: String) -> URL { directory.appendingPathComponent("\(id).json") }
}
