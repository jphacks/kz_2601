// 審査用ビルドでは外す(#319・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import SwiftUI
import UIKit
import MetamonDSL
import MetamonServices
import MetamonRenderer

/// フィードバックの画面を開いた時点の「送れるもの」(#319)。どれも本人が選んだ時だけ送る(既定はすべて送らない)。
/// スクショはメモリにだけ持ち、送らずに閉じれば捨てる。
struct FeedbackContext: Identifiable {
    let id = UUID()
    var screenshot: UIImage?
    var app: AppSummary?
    var device: FeedbackSubmission.Device

    /// 今のアプリの DSL の見せ方。送るのは `ref`(サーバーが本人の木から構造だけを写す)。
    struct AppSummary {
        var ref: FeedbackSubmission.AppRef
        var name: String
        var screens: [String]
        var entities: [String]
        /// 端末にある同じ版の DSL(整形済み)。無ければ要約だけ見せる
        var json: String?
    }

    @MainActor
    static func capture(runtime rt: AppRuntime, screenshot: UIImage?) -> FeedbackContext {
        FeedbackContext(screenshot: screenshot, app: appSummary(rt.document), device: currentDevice())
    }

    static func appSummary(_ doc: AppDocument, cache: DocumentCache = .shared) -> AppSummary? {
        guard doc.version >= 1, !doc.dsl.isVoiceOnly else { return nil } // 初期状態(音声画面だけ)は送る意味が無い
        return AppSummary(
            ref: .init(appId: doc.appId, dslVersion: doc.version),
            name: doc.dsl.app.name,
            screens: doc.dsl.screens.map { $0.title.isEmpty ? $0.id : $0.title },
            entities: doc.dsl.entities.map { e in "\(e.name.isEmpty ? e.id : e.name)(\(e.fields.map(\.name).joined(separator: ", ")))" },
            json: cachedJSON(appId: doc.appId, version: doc.version, cache: cache))
    }

    /// `DocumentCache` の封筒 `{appId, version, dsl}` から、同じ版の `dsl` だけを整形して返す。記録は入っていない。
    static func cachedJSON(appId: String, version: Int, cache: DocumentCache) -> String? {
        guard let data = cache.loadData(appId: appId),
              let env = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              (env["version"] as? Int) == version, let dsl = env["dsl"],
              let out = try? JSONSerialization.data(withJSONObject: dsl, options: [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes])
        else { return nil }
        return String(decoding: out, as: UTF8.self)
    }

    @MainActor
    static func currentDevice() -> FeedbackSubmission.Device {
        let info = Bundle.main.infoDictionary ?? [:]
        return .init(appVersion: info["CFBundleShortVersionString"] as? String ?? "-",
                     build: info["CFBundleVersion"] as? String ?? "-",
                     os: "\(UIDevice.current.systemName) \(UIDevice.current.systemVersion)",
                     model: modelIdentifier(),
                     locale: LanguagePreference.shared.code)
    }

    /// 機種の識別子(例 iPhone17,1)。シミュレータは模している機種。
    static func modelIdentifier() -> String {
        if let sim = ProcessInfo.processInfo.environment["SIMULATOR_MODEL_IDENTIFIER"] { return sim }
        var sys = utsname()
        uname(&sys)
        return withUnsafeBytes(of: &sys.machine) { raw in
            String(decoding: raw.prefix { $0 != 0 }, as: UTF8.self)
        }
    }
}

/// スクショの撮り方と縮め方。
@MainActor
enum FeedbackScreenshot {
    /// 長い辺の上限(px)。文字が読める程度に縮めて送る量を減らす
    static let maxPixels: CGFloat = 1280

    private static var keyWindow: UIWindow? {
        UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .flatMap(\.windows)
            .first { $0.isKeyWindow }
    }

    /// 見えている画面全体(シートも含む)。端末を振った時用。
    static func captureWindow() -> UIImage? {
        keyWindow.flatMap { render($0) }
    }

    /// シートの下のアプリの画面(設定から開いた時用)。シートは root の view の外に載るので写らない。
    static func captureAppScreen() -> UIImage? {
        keyWindow?.rootViewController?.view.flatMap { render($0) }
    }

    private static func render(_ view: UIView) -> UIImage? {
        let size = view.bounds.size
        guard size.width > 0, size.height > 0 else { return nil }
        let format = UIGraphicsImageRendererFormat()
        format.scale = min(view.window?.screen.scale ?? 2, maxPixels / max(size.width, size.height))
        format.opaque = true
        return UIGraphicsImageRenderer(bounds: view.bounds, format: format).image { _ in
            view.drawHierarchy(in: view.bounds, afterScreenUpdates: false)
        }
    }

    /// 上限のバイト数以下の JPEG にする(画質 → 大きさの順に落とす)。無理なら nil。
    nonisolated static func jpeg(_ image: UIImage, maxBytes: Int = FeedbackConsent.maxScreenshotBytes) -> Data? {
        var img = image
        for _ in 0..<4 {
            for q in [0.7, 0.5, 0.35, 0.2] as [CGFloat] {
                if let data = img.jpegData(compressionQuality: q), data.count <= maxBytes { return data }
            }
            let next = CGSize(width: img.size.width * 0.7, height: img.size.height * 0.7)
            let format = UIGraphicsImageRendererFormat()
            format.scale = img.scale
            img = UIGraphicsImageRenderer(size: next, format: format).image { _ in img.draw(in: CGRect(origin: .zero, size: next)) }
        }
        return nil
    }
}
#endif
