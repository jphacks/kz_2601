import AppKit
import ImageIO
import SwiftUI
import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// デモの DSL を Mac の大きさ 3 つ × ライト / ダークで描き、`__Snapshots__/` の PNG と比べる(#1136)。
/// 配置の計算(`GridMetrics.resolveDesktop`)は MetamonRendererTests で見ている。ここは描いた結果の崩れ(重なり・切れ)を見る。
///
/// 既定では飛ばす(CI と手元で macOS / フォントの版が違うと文字の描き方が変わり、毎回落ちるため)。
///   比べる:   TEST_RUNNER_METAMON_SNAPSHOTS=1 xcodebuild -scheme MetamonMac test -only-testing:MetamonMacTests/MacSnapshotTests ...
///   撮り直す: TEST_RUNNER_METAMON_SNAPSHOT_RECORD=1 xcodebuild ...(同じコマンド)
/// xcodebuild は `TEST_RUNNER_` を外した名前でテストに渡す。署名したサンドボックスのホストからは repo に書けないので CODE_SIGNING_ALLOWED=NO で回す。
@MainActor
final class MacSnapshotTests: XCTestCase {
    /// 小さいウィンドウ・最初の大きさ・全画面(14 インチの既定解像度)と同じ大きさの描画域に、最初の画面(`ScreenView`)を描く
    static let sizes: [CGSize] = [CGSize(width: 720, height: 480), CGSize(width: 1100, height: 720), CGSize(width: 1512, height: 945)]

    private var savedLanguage: AppLanguage?
    private var savedAppearance: NSAppearance?

    override func setUpWithError() throws {
        try XCTSkipUnless(SnapshotMode.current != .off,
                          "スナップショットは METAMON_SNAPSHOTS=1(比べる)か METAMON_SNAPSHOT_RECORD=1(撮り直す)の時だけ")
        // 手元の設定で絵が変わらないようにそろえる: 文言は日本語(開発言語)・スクロールバーは重ねる表示
        savedLanguage = LanguagePreference.shared.setting
        savedAppearance = NSApp.appearance
        LanguagePreference.shared.setting = .ja
        try XCTSkipUnless(SnapshotRenderer.forceOverlayScrollers(),
                          "スクロールバーを重ねる表示にできなかった(マウスをつないだ Mac の「常に表示」だと列の幅が変わる)")
    }

    override func tearDown() {
        if let savedLanguage { LanguagePreference.shared.setting = savedLanguage }
        NSApp.appearance = savedAppearance
        super.tearDown()
    }

    func testKakeibo() throws { try verify("kakeibo") }
    func testBMI() throws { try verify("bmi") }
    func testBoard() throws { try verify("board") }
    func testShopping() throws { try verify("shopping") }
    func testWarikan() throws { try verify("warikan") }
    func testRecipes() throws { try verify("recipes") }
    func testGalleryInput() throws { try verify("gallery-input") }
    func testGalleryDisplay() throws { try verify("gallery-display") }

    // MARK: -

    private func verify(_ example: String, file: StaticString = #filePath, line: UInt = #line) throws {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: example, withExtension: "json"),
                                "docs/dsl/examples の \(example).json がアプリに入っていない")
        let dsl = try AppDSL.load(contentsOf: url)
        func view() -> some View {
            let runtime = MacAppRuntime(appId: "snapshot-\(example)-\(UUID().uuidString.lowercased())", dsl: dsl,
                                        ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
            // 最初の画面の中身だけを描く。タブ・ナビゲーションの帯(AppKit の部品)は cacheDisplay に写ったり写らなかったりするので入れない
            return ScreenView(route: Route(screen: runtime.currentScreen, stack: runtime.topStack))
                .environment(\.dslRuntime, runtime)
                .environment(\.componentRegistry, ComponentRegistry.standard)
                .environment(\.openForms, runtime.openForms)
                .environment(\.locale, Locale(identifier: "ja_JP"))
        }
        var failures: [String] = []
        for size in Self.sizes {
            for dark in [false, true] {
                let name = "\(example)_\(Int(size.width))x\(Int(size.height))_\(dark ? "dark" : "light")"
                let image = try XCTUnwrap(SnapshotRenderer.render(view(), size: size, dark: dark), "\(name) を描けなかった")
                if let message = try SnapshotStore.check(image, name: name) { failures.append(message) }
            }
        }
        if !failures.isEmpty { XCTFail(failures.joined(separator: "\n"), file: file, line: line) }
    }
}

/// 撮り直すか・比べるか・飛ばすか(環境変数)
enum SnapshotMode {
    case off, compare, record

    static var current: SnapshotMode {
        let env = ProcessInfo.processInfo.environment
        if env["METAMON_SNAPSHOT_RECORD"] == "1" { return .record }
        if env["METAMON_SNAPSHOTS"] == "1" { return .compare }
        return .off
    }
}

/// SwiftUI をウインドウに載せ、出現のアニメーションが終わってから写す(`ImageRenderer` は NavigationStack・onAppear を描けない)
@MainActor
enum SnapshotRenderer {
    /// 1pt = 1px で保存する(Retina の 2 倍は PNG が 4 倍になる。配置の崩れは 1 倍で見える)
    static let scale: CGFloat = 1
    /// 出現(index × 0.03 秒の遅延 + 0.32 秒)が最大 0.68 秒。余裕を見る
    static let settle: TimeInterval = 1.2
    static let appearanceSettle: TimeInterval = 0.5

    /// 「スクロールバーを表示: 常に」(マウスをつないだ Mac の既定)だと、スクロールする部品がバーの幅だけ細くなる。
    /// このプロセスだけ(引数のドメイン・保存しない)重ねる表示にして、AppKit に読み直させる
    static func forceOverlayScrollers() -> Bool {
        if NSScroller.preferredScrollerStyle == .overlay { return true }
        var args = UserDefaults.standard.volatileDomain(forName: UserDefaults.argumentDomain)
        args["AppleShowScrollBars"] = "WhenScrolling"
        UserDefaults.standard.setVolatileDomain(args, forName: UserDefaults.argumentDomain)
        DistributedNotificationCenter.default().postNotificationName(NSNotification.Name("AppleShowScrollBarsSettingChanged"),
                                                                     object: nil, userInfo: nil, deliverImmediately: true)
        let deadline = Date().addingTimeInterval(2)
        while NSScroller.preferredScrollerStyle != .overlay, Date() < deadline {
            RunLoop.main.run(until: Date().addingTimeInterval(0.05))
        }
        return NSScroller.preferredScrollerStyle == .overlay
    }

    static func render<V: View>(_ view: V, size: CGSize, dark: Bool) -> CGImage? {
        let appearance = NSAppearance(named: dark ? .darkAqua : .aqua)!
        // AppKit の部品(タブの切り替えなど)はアプリの見た目で描かれるので、アプリごと切り替える(戻すのは tearDown)。
        // 切り替えは少し遅れて行き渡るので、待ってから作る(待たないと前の絵の色のタブが写る)
        if NSApp.appearance?.name != appearance.name {
            NSApp.appearance = appearance
            RunLoop.main.run(until: Date().addingTimeInterval(appearanceSettle))
        }
        let host = NSHostingView(rootView: view.frame(width: size.width, height: size.height)
            .environment(\.colorScheme, dark ? .dark : .light))
        host.frame = CGRect(origin: .zero, size: size)
        host.appearance = appearance
        // 画面の外に置いたウインドウ(作業の邪魔をしない・onAppear を走らせるため実際に出す)
        let window = NSWindow(contentRect: CGRect(x: -20_000, y: -20_000, width: size.width, height: size.height),
                              styleMask: [.borderless], backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        window.appearance = appearance
        window.contentView = host
        window.orderFrontRegardless()
        defer { window.orderOut(nil); window.contentView = nil }
        RunLoop.main.run(until: Date().addingTimeInterval(settle))
        host.layoutSubtreeIfNeeded()

        guard let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: Int(size.width * scale), pixelsHigh: Int(size.height * scale),
                                         bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                                         colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0) else { return nil }
        rep.size = size
        appearance.performAsCurrentDrawingAppearance {
            host.cacheDisplay(in: host.bounds, to: rep)
        }
        return rep.cgImage
    }
}

/// 参照の PNG の置き場と比べ方
enum SnapshotStore {
    /// 1 画素の色の差(0〜255 の各成分の最大)がこれ以下なら同じとみなす(文字のアンチエイリアスの揺れ)
    static let pixelTolerance = 32
    /// 違う画素の割合がこれ以下なら通す(0.5%)
    static let maxDiffRatio = 0.005

    static var directory: URL {
        URL(fileURLWithPath: #filePath).deletingLastPathComponent().appendingPathComponent("__Snapshots__", isDirectory: true)
    }

    /// 一致なら nil・違えば失敗の文言(撮り直す時は書いて nil)
    static func check(_ image: CGImage, name: String) throws -> String? {
        let reference = directory.appendingPathComponent("\(name).png")
        if SnapshotMode.current == .record {
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
            try png(image).write(to: reference)
            return nil
        }
        guard let data = try? Data(contentsOf: reference), let rep = NSBitmapImageRep(data: data), let expected = rep.cgImage else {
            return "\(name): 参照の PNG が無い(METAMON_SNAPSHOT_RECORD=1 で撮る)"
        }
        let diff = SnapshotDiff.compare(expected, image, pixelTolerance: pixelTolerance)
        guard !diff.passes(maxRatio: maxDiffRatio) else { return nil }
        let out = FileManager.default.temporaryDirectory.appendingPathComponent("MetamonSnapshots", isDirectory: true)
        try? FileManager.default.createDirectory(at: out, withIntermediateDirectories: true)
        let actual = out.appendingPathComponent("\(name).actual.png")
        try? png(image).write(to: actual)
        return "\(name): \(diff.summary)(実際の絵: \(actual.path))"
    }

    /// 不透明な RGB の PNG にする(アルファを捨てて小さく)
    static func png(_ image: CGImage) throws -> Data {
        let w = image.width, h = image.height
        guard let space = CGColorSpace(name: CGColorSpace.sRGB),
              let ctx = CGContext(data: nil, width: w, height: h, bitsPerComponent: 8, bytesPerRow: 0, space: space,
                                  bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue) else { throw CocoaError(.fileWriteUnknown) }
        ctx.draw(image, in: CGRect(x: 0, y: 0, width: w, height: h))
        let data = NSMutableData()
        guard let opaque = ctx.makeImage(),
              let dest = CGImageDestinationCreateWithData(data, "public.png" as CFString, 1, nil) else { throw CocoaError(.fileWriteUnknown) }
        CGImageDestinationAddImage(dest, opaque, nil)
        guard CGImageDestinationFinalize(dest) else { throw CocoaError(.fileWriteUnknown) }
        return data as Data
    }
}

/// 2 枚の絵の画素の差(sRGB の RGBA8 にそろえてから比べる)
struct SnapshotDiff: Equatable {
    var sizeMismatch: Bool
    var differing: Int
    var total: Int

    var ratio: Double { total == 0 ? 0 : Double(differing) / Double(total) }

    func passes(maxRatio: Double) -> Bool { !sizeMismatch && ratio <= maxRatio }

    var summary: String {
        sizeMismatch ? "大きさが違う" : String(format: "%d / %d 画素(%.2f%%)が違う", differing, total, ratio * 100)
    }

    static func compare(_ a: CGImage, _ b: CGImage, pixelTolerance: Int) -> SnapshotDiff {
        guard a.width == b.width, a.height == b.height, let pa = rgba(a), let pb = rgba(b) else {
            return SnapshotDiff(sizeMismatch: true, differing: 0, total: 0)
        }
        var differing = 0
        var i = 0
        while i < pa.count {
            var worst = 0
            for c in 0..<4 { worst = max(worst, abs(Int(pa[i + c]) - Int(pb[i + c]))) }
            if worst > pixelTolerance { differing += 1 }
            i += 4
        }
        return SnapshotDiff(sizeMismatch: false, differing: differing, total: a.width * a.height)
    }

    static func rgba(_ image: CGImage) -> [UInt8]? {
        let w = image.width, h = image.height
        var buffer = [UInt8](repeating: 0, count: w * h * 4)
        let drawn = buffer.withUnsafeMutableBytes { raw -> Bool in
            guard let space = CGColorSpace(name: CGColorSpace.sRGB),
                  let ctx = CGContext(data: raw.baseAddress, width: w, height: h, bitsPerComponent: 8, bytesPerRow: w * 4,
                                      space: space, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else { return false }
            ctx.draw(image, in: CGRect(x: 0, y: 0, width: w, height: h))
            return true
        }
        return drawn ? buffer : nil
    }
}
