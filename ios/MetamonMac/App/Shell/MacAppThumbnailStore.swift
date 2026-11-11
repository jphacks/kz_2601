import SwiftUI
import AppKit
import Observation

/// サイドバーの行に出すアプリの画面の写し(#1281)。iOS の `AppThumbnailStore` の Mac 版。
/// `Application Support/metamon-thumbs/<appId>.jpg` に 1/3 の大きさで残す。アプリを消したら消す。
/// 撮るのは中央の列(`MacThumbnailAnchor` を付けた所)。`ImageRenderer` は NavigationStack などを描けないので、ウインドウの描画を写す。
@MainActor
@Observable
final class MacAppThumbnailStore {
    static let shared = MacAppThumbnailStore()

    /// 撮るたびに進む。サイドバーはこれを読んで描き直す
    private(set) var revision = 0
    @ObservationIgnored private var memory: [String: NSImage] = [:]
    @ObservationIgnored private let directory: URL

    static let scale: CGFloat = 1.0 / 3.0

    init(directory: URL? = nil) {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        self.directory = directory ?? base.appendingPathComponent("metamon-thumbs", isDirectory: true)
    }

    func image(for appId: String) -> NSImage? {
        if let hit = memory[appId] { return hit }
        guard let data = try? Data(contentsOf: url(appId)), let image = NSImage(data: data) else { return nil }
        memory[appId] = image
        return image
    }

    /// 中央の列を 1/3 の大きさで撮って残す。初期(音声)画面だけのアプリは撮らない(iOS と同じ)
    func capture(appId: String, isVoiceOnly: Bool, anchor: MacThumbnailAnchor) {
        guard !isVoiceOnly, !appId.isEmpty, let image = anchor.snapshot(scale: Self.scale) else { return }
        store(image, for: appId)
    }

    func store(_ image: NSImage, for appId: String) {
        memory[appId] = image
        revision += 1
        guard let data = Self.jpeg(image) else { return }
        let target = url(appId)
        let dir = directory
        Task.detached(priority: .utility) {
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

    private static func jpeg(_ image: NSImage) -> Data? {
        guard let cg = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else { return nil }
        return NSBitmapImageRep(cgImage: cg).representation(using: .jpeg, properties: [.compressionFactor: 0.75])
    }

    func url(_ appId: String) -> URL {
        let safe = appId.map { $0.isLetter || $0.isNumber || $0 == "-" || $0 == "_" ? $0 : "_" }
        return directory.appendingPathComponent("\(String(safe)).jpg")
    }
}

/// 写す範囲の目印。中央の列の背景に `MacThumbnailAnchorView` を置き、その範囲のウインドウの描画を写す。
@MainActor
final class MacThumbnailAnchor {
    weak var view: NSView?

    func snapshot(scale: CGFloat) -> NSImage? {
        guard let view, let root = view.window?.contentView else { return nil }
        let rect = view.convert(view.bounds, to: root)
        guard rect.width >= 1, rect.height >= 1, let rep = root.bitmapImageRepForCachingDisplay(in: rect) else { return nil }
        root.cacheDisplay(in: rect, to: rep)
        guard let full = rep.cgImage else { return nil }
        // 画面の点の 1/3 を、Retina でも滲まないよう 2 倍の画素で持つ(iOS と同じ)
        let size = CGSize(width: (rect.width * scale).rounded(), height: (rect.height * scale).rounded())
        let pixels = CGSize(width: size.width * 2, height: size.height * 2)
        guard let context = CGContext(data: nil, width: Int(pixels.width), height: Int(pixels.height), bitsPerComponent: 8,
                                      bytesPerRow: 0, space: CGColorSpaceCreateDeviceRGB(),
                                      bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue) else { return nil }
        context.interpolationQuality = .high
        context.draw(full, in: CGRect(origin: .zero, size: pixels))
        guard let scaled = context.makeImage() else { return nil }
        return NSImage(cgImage: scaled, size: size)
    }
}

/// 目印の view。押せない・見えない
struct MacThumbnailAnchorView: NSViewRepresentable {
    let anchor: MacThumbnailAnchor

    func makeNSView(context: Context) -> NSView {
        let view = PassthroughView()
        anchor.view = view
        return view
    }

    func updateNSView(_ view: NSView, context: Context) {
        anchor.view = view
    }

    private final class PassthroughView: NSView {
        override func hitTest(_ point: NSPoint) -> NSView? { nil }
    }
}

/// サイドバーの行の小さな写し。無ければ名前の頭の文字(iOS のカードと同じブランドの色)
struct SidebarThumbnail: View {
    let name: String
    let image: NSImage?

    static let size = CGSize(width: 40, height: 28)

    var body: some View {
        Group {
            if let image {
                // 画面の上(タイトル)が見えるよう上揃えで切り出す
                Image(nsImage: image)
                    .resizable()
                    .scaledToFill()
                    .frame(width: Self.size.width, height: Self.size.height, alignment: .top)
            } else {
                ZStack {
                    Theme.brandGradient
                    Text(initial)
                        .font(.system(size: 13, weight: .bold, design: .rounded))
                        .foregroundStyle(.white)
                }
            }
        }
        .frame(width: Self.size.width, height: Self.size.height)
        .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 5, style: .continuous).strokeBorder(.separator))
        .accessibilityHidden(true)
    }

    private var initial: String {
        String(name.trimmingCharacters(in: .whitespaces).prefix(1))
    }
}
