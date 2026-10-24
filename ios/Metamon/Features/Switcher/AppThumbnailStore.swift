import SwiftUI
import UIKit
import Observation

/// 変身一覧カードのスクショサムネ。`Application Support/metamon-thumbs/<appId>.jpg` に 1/3 スケールで保存する。
/// 撮るタイミング: 切替・一覧を開く直前(今の画面)/ apply 後(`RootView` が version 変化の 1.2 秒後に呼ぶ)。
///
/// 画面の取得はキーウィンドウの `drawHierarchy`。`ImageRenderer` は NavigationStack / TabView(UIKit 製)を描けず空になるため使わない。
@MainActor
@Observable
final class AppThumbnailStore {
    static let shared = AppThumbnailStore()

    /// 撮るたびに進む。カードはこれを読んで再描画する。
    private(set) var revision = 0
    @ObservationIgnored private var memory: [String: UIImage] = [:]
    @ObservationIgnored private let directory: URL

    static let scale: CGFloat = 1.0 / 3.0

    init(directory: URL? = nil) {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        self.directory = directory ?? base.appendingPathComponent("metamon-thumbs", isDirectory: true)
    }

    func image(for appId: String) -> UIImage? {
        if let hit = memory[appId] { return hit }
        guard let data = try? Data(contentsOf: url(appId)), let img = UIImage(data: data) else { return nil }
        memory[appId] = img
        return img
    }

    /// 今のキーウィンドウを 1/3 スケールで撮って保存する。初期(音声)画面は撮らない(カードに出す意味が無い)。
    func capture(appId: String, isVoiceOnly: Bool) {
        guard !isVoiceOnly, !appId.isEmpty else { return }
        guard let window = UIApplication.shared.connectedScenes
            .compactMap({ $0 as? UIWindowScene })
            .flatMap(\.windows)
            .first(where: \.isKeyWindow) else { return }
        // ステータスバー(時計・電池)は切り落とし、アプリのタイトル行から下を撮る
        let bounds = window.bounds
        let top = window.safeAreaInsets.top
        let size = CGSize(width: bounds.width * Self.scale, height: (bounds.height - top) * Self.scale)
        let format = UIGraphicsImageRendererFormat()
        format.scale = 2
        format.opaque = true
        let image = UIGraphicsImageRenderer(size: size, format: format).image { _ in
            let full = CGRect(x: 0, y: -top * Self.scale, width: bounds.width * Self.scale, height: bounds.height * Self.scale)
            window.drawHierarchy(in: full, afterScreenUpdates: false)
        }
        memory[appId] = image
        revision += 1
        let target = url(appId)
        let dir = directory
        Task.detached(priority: .utility) {
            guard let data = image.jpegData(compressionQuality: 0.75) else { return }
            do {
                try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
                try data.write(to: target, options: .atomic)
            } catch {
                print("[Metamon] thumbnail store failed: \(error)")
            }
        }
    }

    func remove(appId: String) {
        memory[appId] = nil
        try? FileManager.default.removeItem(at: url(appId))
        revision += 1
    }

    private func url(_ appId: String) -> URL {
        let safe = appId.map { $0.isLetter || $0.isNumber || $0 == "-" || $0 == "_" ? $0 : "_" }
        return directory.appendingPathComponent("\(String(safe)).jpg")
    }
}
