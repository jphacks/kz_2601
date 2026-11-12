// 審査用ビルドでは外す(#319・iOS と同じ)。
#if !APP_STORE_REVIEW
import AppKit
import MetamonDSL
import MetamonServices

/// フィードバックの画面を開いた時点の「送れるもの」(#983・iOS の `FeedbackContext`)。どれも本人が選んだ時だけ送る。
/// スクショはメモリにだけ持ち、送らずに閉じれば捨てる。
/// 今のアプリの設計(DSL・#1330)は、そのアプリがサーバーにある時だけ付けられる(送るのは appId と版だけで、
/// サーバーが本人の木から構造を写す・iOS と同じ)。端末だけのアプリ・同梱の例は付けない。
struct MacFeedbackContext: Identifiable {
    let id = UUID()
    var screenshot: CGImage?
    var app: AppSummary?
    var device: FeedbackSubmission.Device

    /// 今のアプリの DSL の見せ方(iOS の `FeedbackContext.AppSummary` と同じ)。送るのは `ref`
    struct AppSummary: Equatable {
        var ref: FeedbackSubmission.AppRef
        var name: String
        var screens: [String]
        var entities: [String]
        /// 端末にある同じ版の DSL(整形済み)。無ければ要約だけ見せる
        var json: String?
    }

    /// 中央に出しているアプリ(`MacRootView` が入れる)。サーバーにあるアプリの時だけ
    struct CurrentApp {
        var document: AppDocument
        var envelope: Data?
    }

    @MainActor
    static func capture(screenshot: CGImage?, app: CurrentApp? = nil) -> MacFeedbackContext {
        MacFeedbackContext(screenshot: screenshot, app: app.flatMap { appSummary($0.document, envelope: $0.envelope) },
                           device: currentDevice())
    }

    /// iOS の `FeedbackContext.appSummary` と同じ見せ方。初期状態(音声画面だけ)・版が無いものは送る意味が無いので nil
    static func appSummary(_ doc: AppDocument, envelope: Data?) -> AppSummary? {
        guard doc.version >= 1, !doc.dsl.isVoiceOnly else { return nil }
        return AppSummary(
            ref: .init(appId: doc.appId, dslVersion: doc.version),
            name: doc.dsl.app.name,
            screens: doc.dsl.screens.map { $0.title.isEmpty ? $0.id : $0.title },
            entities: doc.dsl.entities.map { e in "\(e.name.isEmpty ? e.id : e.name)(\(e.fields.map(\.name).joined(separator: ", ")))" },
            json: envelope.flatMap { prettyDSL($0, version: doc.version) })
    }

    /// 封筒 `{appId, version, dsl}` から、同じ版の `dsl` だけを整形して返す(iOS の `cachedJSON` と同じ)。記録は入っていない
    static func prettyDSL(_ envelope: Data, version: Int) -> String? {
        guard let env = try? JSONSerialization.jsonObject(with: envelope) as? [String: Any],
              (env["version"] as? Int) == version, let dsl = env["dsl"],
              let out = try? JSONSerialization.data(withJSONObject: dsl, options: [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes])
        else { return nil }
        return String(decoding: out, as: UTF8.self)
    }

    @MainActor
    static func currentDevice() -> FeedbackSubmission.Device {
        FeedbackCopy.device(os: osName(ProcessInfo.processInfo.operatingSystemVersion), model: modelIdentifier())
    }

    /// 例 "macOS 15.1.0"。サーバーは先頭の名前で iOS と分ける。
    static func osName(_ v: OperatingSystemVersion) -> String {
        "macOS \(v.majorVersion).\(v.minorVersion).\(v.patchVersion)"
    }

    /// 機種の識別子(例 Mac15,3・MacBookPro18,3)。`hw.model` はシリアル番号ではない。
    static func modelIdentifier() -> String {
        var size = 0
        guard sysctlbyname("hw.model", nil, &size, nil, 0) == 0, size > 0 else { return "Mac" }
        var buf = [CChar](repeating: 0, count: size)
        guard sysctlbyname("hw.model", &buf, &size, nil, 0) == 0 else { return "Mac" }
        let s = String(decoding: buf.prefix { $0 != 0 }.map { UInt8(bitPattern: $0) }, as: UTF8.self)
        return s.isEmpty ? "Mac" : s
    }
}

/// スクショの撮り方と縮め方(#983)。自分のウインドウの中身を描き直して撮るので、画面収録の許可は要らない。
@MainActor
enum MacFeedbackScreenshot {
    /// 長い辺の上限(px)。iOS と同じ
    static let maxPixels: CGFloat = 1280

    /// 開いているアプリのウインドウ(`MacCommands.mainWindowId` のもの)のうち一番前。設定・フィードバックのウインドウは撮らない。
    /// アプリのウインドウが無ければ撮らない(スクショの添付は出さない)。
    static func captureAppWindow() -> CGImage? {
        let window = NSApp.orderedWindows.first { w in
            w.isVisible && !w.isMiniaturized && w.identifier?.rawValue.hasPrefix(MacCommands.mainWindowId) == true
        }
        return window?.contentView.flatMap { render($0) }
    }

    static func render(_ view: NSView) -> CGImage? {
        let bounds = view.bounds
        guard bounds.width > 0, bounds.height > 0,
              let rep = view.bitmapImageRepForCachingDisplay(in: bounds) else { return nil }
        view.cacheDisplay(in: bounds, to: rep)
        guard let full = rep.cgImage else { return nil }
        return scaled(full, maxPixels: maxPixels)
    }

    /// 長い辺が `maxPixels` 以下になるよう縮める(小さければそのまま)。
    nonisolated static func scaled(_ image: CGImage, maxPixels: CGFloat) -> CGImage? {
        let w = CGFloat(image.width), h = CGFloat(image.height)
        let ratio = min(1, maxPixels / max(w, h))
        return ratio >= 1 ? image : resized(image, by: ratio)
    }

    nonisolated private static func resized(_ image: CGImage, by ratio: CGFloat) -> CGImage? {
        let w = max(1, Int(CGFloat(image.width) * ratio)), h = max(1, Int(CGFloat(image.height) * ratio))
        guard let ctx = CGContext(data: nil, width: w, height: h, bitsPerComponent: 8, bytesPerRow: 0,
                                  space: CGColorSpace(name: CGColorSpace.sRGB) ?? CGColorSpaceCreateDeviceRGB(),
                                  bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue) else { return nil }
        ctx.interpolationQuality = .high
        ctx.draw(image, in: CGRect(x: 0, y: 0, width: w, height: h))
        return ctx.makeImage()
    }

    /// 上限のバイト数以下の JPEG にする(画質 → 大きさの順に落とす・iOS と同じ)。無理なら nil。
    nonisolated static func jpeg(_ image: CGImage, maxBytes: Int = FeedbackConsent.maxScreenshotBytes) -> Data? {
        var img = image
        for _ in 0..<4 {
            let rep = NSBitmapImageRep(cgImage: img)
            for q in [0.7, 0.5, 0.35, 0.2] {
                if let data = rep.representation(using: .jpeg, properties: [.compressionFactor: q]), data.count <= maxBytes {
                    return data
                }
            }
            guard let next = resized(img, by: 0.7) else { return nil }
            img = next
        }
        return nil
    }
}
#endif
