#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif
import PencilKit
import Observation
import MetamonDSL

/// `drawingField` の絵を photo field の値にする。写真と同じ "data:image/jpeg;base64,..."(800px 以下)なので、
/// 一覧・image・共有・同期は写真と同じ扱いになる。白地に描く(JPEG は透明を持てない・ダークモードでも同じ見た目)。
public enum DrawingCodec {
    /// 線がくっきり残るよう写真(q0.5)より少し高い品質にする。800px の線画でおよそ 30〜120KB(Firestore の 1MB に収まる)。
    public static let quality: CGFloat = 0.7
    public static let maxSide: CGFloat = 800

    /// 描いた線 + 下絵(保存済みの絵に描き足す時)→ field の値。何も無ければ `.null`(field を空にする)。
    public static func fieldValue(drawing: PKDrawing, background: PlatformImage?, canvasSize: CGSize) -> JSONValue {
        dataURL(drawing: drawing, background: background, canvasSize: canvasSize).map(JSONValue.string) ?? .null
    }

    public static func dataURL(drawing: PKDrawing, background: PlatformImage?, canvasSize: CGSize) -> String? {
        guard let image = render(drawing: drawing, background: background, canvasSize: canvasSize) else { return nil }
        return PhotoCodec.dataURL(from: image, maxSide: maxSide, quality: quality)
    }

    /// キャンバスの見えている範囲をそのまま 1 枚に。下絵はキャンバスに収まるように(aspect fit)置く。
    static func render(drawing: PKDrawing, background: PlatformImage?, canvasSize: CGSize) -> PlatformImage? {
        guard !drawing.strokes.isEmpty || background != nil, canvasSize.width >= 1, canvasSize.height >= 1 else { return nil }
        let rect = CGRect(origin: .zero, size: canvasSize)
        // 長い辺が maxSide を少し超える倍率で描き、PhotoCodec が 800px に縮める(線がにじまない)
        let scale = min(3, max(1, (maxSide / max(canvasSize.width, canvasSize.height)).rounded(.up)))
        // インクの色はダークモードで反転するので、常にライトで描く(保存する絵は白地に黒)
        var ink: PlatformImage?
        #if canImport(UIKit)
        UITraitCollection(userInterfaceStyle: .light).performAsCurrent {
            ink = drawing.image(from: rect, scale: scale)
        }
        #else
        (NSAppearance(named: .aqua) ?? NSAppearance.currentDrawing()).performAsCurrentDrawingAppearance {
            ink = drawing.image(from: rect, scale: scale)
        }
        #endif
        return PlatformGraphics.image(size: canvasSize, scale: scale, opaque: true) { ctx in
            PlatformColor.white.setFill()
            ctx.fill(rect)
            if let background { background.drawUpright(in: aspectFit(background.size, in: rect)) }
            ink?.drawUpright(in: rect)
        }
    }

    static func aspectFit(_ size: CGSize, in rect: CGRect) -> CGRect {
        guard size.width > 0, size.height > 0 else { return rect }
        let r = min(rect.width / size.width, rect.height / size.height)
        let fit = CGSize(width: size.width * r, height: size.height * r)
        return CGRect(x: rect.midX - fit.width / 2, y: rect.midY - fit.height / 2, width: fit.width, height: fit.height)
    }
}

/// 描いている間の状態(キャンバスの外に持つのでテストできる)。完了で field に入れ、キャンセルなら捨てる。
@MainActor
@Observable
final class DrawingSession: Identifiable {
    enum Tool: Hashable { case pen, eraser }

    let id = UUID()

    var drawing = PKDrawing()
    /// 保存済みの絵(描き足す時の下絵)。「全部消す」で消える。
    var background: PlatformImage?
    var tool: Tool = .pen
    /// 「全部消す」のたびに増える(キャンバスに空の絵を戻す合図)
    private(set) var revision = 0

    init(value: JSONValue) {
        background = PhotoCodec.image(from: value)
    }

    var isEmpty: Bool { drawing.strokes.isEmpty && background == nil }

    /// VoiceOver で読むキャンバスの値(iOS・Mac 共通)。
    var accessibilityValue: String {
        let n = drawing.strokes.count
        return n == 0 ? L("何も描いていません") : L("線 \(n) 本")
    }

    /// Mac のキャンバスで描き終えた線を足す(iOS は PKCanvasView の絵をそのまま入れる)。
    func add(_ stroke: PKStroke) {
        drawing.strokes.append(stroke)
    }

    /// 「全部消す」: 描いた線も下絵も消す。
    func clear() {
        drawing = PKDrawing()
        background = nil
        revision += 1
    }

    /// 完了: 絵を field に入れる(何も無ければ空にする)。
    func commit(to form: FormDraft, field: String, canvasSize: CGSize) {
        form.values[field] = DrawingCodec.fieldValue(drawing: drawing, background: background, canvasSize: canvasSize)
    }
}
