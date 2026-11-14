import Foundation
import UniformTypeIdentifiers
import MetamonDSL

/// videoField にファイルから入れる動画(#1339・Mac のファイル選択とドラッグ&ドロップ)。
/// 取り込み(mp4 化・先頭 maxDuration 秒・サムネイル)は `VideoCodec.ingest` をそのまま使う。
enum VideoFileImport {
    /// 取り込む元。一時ファイルは取り込んだら消す。ユーザーのファイルは読むだけで消さない。
    enum Origin: Equatable {
        case temporary
        case userFile
    }

    /// ファイル選択に出す種類。
    static let contentTypes: [UTType] = [.movie, .mpeg4Movie, .quickTimeMovie]

    /// 動画として受け付けるか(拡張子で見る。ドロップされた URL の判定にも使う)。
    static func accepts(_ url: URL) -> Bool {
        guard url.isFileURL, let type = UTType(filenameExtension: url.pathExtension.lowercased()) else { return false }
        return contentTypes.contains { type.conforms(to: $0) }
    }

    /// 取り込み後に元を消すか。
    static func removesSource(_ origin: Origin) -> Bool { origin == .temporary }

    /// 元から取り込む。ユーザーのファイルは取り込み(書き出し)が終わるまで security-scoped のアクセスを開き、終わったら閉じる。
    static func ingest(
        _ url: URL,
        origin: Origin,
        maxDuration: Int,
        store: LocalVideoStore = .shared
    ) async throws -> VideoValue {
        let scoped = origin == .userFile && url.startAccessingSecurityScopedResource()
        defer {
            if scoped { url.stopAccessingSecurityScopedResource() }
            if removesSource(origin) { try? FileManager.default.removeItem(at: url) }
        }
        return try await VideoCodec.ingest(url, maxDuration: maxDuration, store: store)
    }
}
