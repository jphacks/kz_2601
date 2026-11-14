import Foundation
import UniformTypeIdentifiers

/// photoField にファイル・ドロップ・貼り付けから入れる画像(Mac・#1035)。
/// どれも撮影・写真選択と同じ `PhotoCodec`(800px 以下・JPEG)の data URL にする。元のファイルは読むだけで消さない。
@MainActor
enum PhotoFileImport {
    /// ファイル選択に出す種類。
    static let contentTypes: [UTType] = [.image]

    /// 画像として受け付けるか(拡張子で見る。ドロップされた URL の判定にも使う)。
    static func accepts(_ url: URL) -> Bool {
        guard url.isFileURL, let type = UTType(filenameExtension: url.pathExtension.lowercased()) else { return false }
        return contentTypes.contains { type.conforms(to: $0) }
    }

    /// ファイルの画像を読む。ファイル選択の URL は security-scoped のアクセスを開いてから読む。
    static func dataURL(fileAt url: URL) -> String? {
        guard accepts(url) else { return nil }
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        guard let data = try? Data(contentsOf: url) else { return nil }
        return PhotoCodec.dataURL(from: data)
    }

    /// クリップボードの画像(⌘V・「貼り付け」)。画像が無ければ nil。
    static func dataURL(pasteboard board: PlatformPasteboard.Board = .general) -> String? {
        PlatformPasteboard.image(on: board).flatMap { PhotoCodec.dataURL(from: $0) }
    }
}
