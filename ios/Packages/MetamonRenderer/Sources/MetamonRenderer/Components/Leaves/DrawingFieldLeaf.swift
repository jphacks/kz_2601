import SwiftUI
import PencilKit
import MetamonDSL

/// `drawingField`: 指・Apple Pencil で手書きして photo field に入れる(PencilKit)。
/// フォームには描いた絵(か「タップして描く」の枠)を出し、タップで全画面のキャンバスを開く
/// (フォームのスクロールと描く指がぶつからない)。完了で `DrawingCodec` が写真と同じ JPEG にして field に入れる。
/// 審査用ビルドにも入れる(カメラ・写真ライブラリ・マイクを使わない)。
struct DrawingFieldLeaf: View {
    let c: Component
    let ctx: RenderContext

    var body: some View {
        if let form = ctx.form, let field = c.field {
            DrawingFieldPanel(c: c, form: form, field: field)
        } else {
            NeedsForm(c: c)
        }
    }
}

struct DrawingFieldPanel: View {
    let c: Component
    let form: FormDraft
    let field: String

    @State private var session: DrawingSession?
    @Environment(\.dynamicTypeSize) private var typeSize

    private var title: String { c.label.flatMap { $0.isEmpty ? nil : $0 } ?? L("手書き") }
    private var value: JSONValue { form.values[field] ?? .null }
    private var picture: PlatformImage? { PhotoCodec.image(from: value) }
    private var hasDrawing: Bool { picture != nil || ImageHostPolicy.loadableURL(value.stringValue) != nil }

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            Text(title)
                .font(RenderTheme.Fonts.inputLabel)
                .foregroundStyle(.secondary)
                .padding(.leading, 2)
                .accessibilityHidden(true)
            Button { open() } label: { preview }
                .buttonStyle(.plain)
                .accessibilityLabel(hasDrawing ? L("\(title)(描いた絵あり)") : title)
                .accessibilityHint(Self.openHint)
                .accessibilityIdentifier("drawingField.\(field)")
            if hasDrawing {
                let layout = typeSize.isAccessibilitySize ? AnyLayout(VStackLayout(spacing: RenderTheme.Spacing.sm)) : AnyLayout(HStackLayout(spacing: RenderTheme.Spacing.sm))
                layout {
                    Button { open() } label: {
                        Label(L("描き足す"), systemImage: "pencil.tip").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(RenderButtonStyle(kind: .secondary))
                    Button(role: .destructive) {
                        form.values[field] = .null
                    } label: {
                        Label(L("削除"), systemImage: "trash").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(RenderButtonStyle(kind: .destructive))
                    .accessibilityLabel(L("\(title)を削除"))
                }
            }
        }
        .platformFullScreenCover(item: $session) { s in
            DrawingSheet(session: s, title: title) { size in s.commit(to: form, field: field, canvasSize: size) }
        }
    }

    @ViewBuilder
    private var preview: some View {
        let shape = RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous)
        if let picture {
            Image(platformImage: picture)
                .resizable()
                .scaledToFit()
                .frame(maxWidth: .infinity, minHeight: 100, maxHeight: 220)
                .background(Color.white, in: shape)
                .clipShape(shape)
                .overlay(shape.stroke(RenderTheme.Colors.hairline))
        } else if hasDrawing {
            PhotoView(value: value, height: 160, accessibilityLabel: title)
        } else {
            Label(L("タップして描く"), systemImage: "scribble.variable")
                .font(.body.weight(.medium))
                .foregroundStyle(RenderTheme.Colors.brandText)
                .frame(maxWidth: .infinity, minHeight: 120)
                .background(RenderTheme.Colors.brandSoft, in: shape)
                .overlay(shape.strokeBorder(RenderTheme.Colors.brandSoftStroke, style: StrokeStyle(lineWidth: 1.5, dash: [6, 4])))
        }
    }

    private func open() {
        session = DrawingSession(value: value)
    }

    private static var openHint: String {
        #if os(iOS)
        L("手書きの画面を開きます。指や Apple Pencil で描けます")
        #else
        L("手書きの画面を開きます。マウスやトラックパッドで描けます")
        #endif
    }
}

/// 全画面のキャンバス。下に ペン / 消しゴム・元に戻す・やり直す・全部消す、上に キャンセル / 完了。
struct DrawingSheet: View {
    let session: DrawingSession
    let title: String
    let onDone: @MainActor (CGSize) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var canvas = CanvasHandle()
    @State private var canvasSize: CGSize = .zero
    @State private var confirmClear = false
    @State private var confirmDiscard = false

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                GeometryReader { geo in
                    ZStack {
                        Color.white
                        if let bg = session.background {
                            Image(platformImage: bg).resizable().scaledToFit().accessibilityHidden(true)
                        }
                        #if os(iOS)
                        DrawingCanvas(session: session, handle: canvas)
                        #else
                        MacDrawingCanvas(session: session)
                        #endif
                    }
                    .onAppear { canvasSize = geo.size }
                    .onChange(of: geo.size) { _, s in canvasSize = s }
                }
                .environment(\.colorScheme, .light)
                Divider()
                tools
            }
            .navigationTitle(title)
            .inlineNavigationTitle()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L("キャンセル")) {
                        if session.drawing.strokes.isEmpty { dismiss() } else { confirmDiscard = true }
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("完了")) {
                        onDone(canvasSize)
                        dismiss()
                    }
                    .fontWeight(.semibold)
                }
            }
            .confirmationDialog(L("全部消しますか?"), isPresented: $confirmClear, titleVisibility: .visible) {
                Button(L("全部消す"), role: .destructive) {
                    session.clear()
                    canvas.didClear()
                    PlatformSystem.announce(L("全部消しました"))
                }
                Button(L("キャンセル"), role: .cancel) {}
            }
            .confirmationDialog(L("描いた線を保存せずに閉じますか?"), isPresented: $confirmDiscard, titleVisibility: .visible) {
                Button(L("保存せずに閉じる"), role: .destructive) { dismiss() }
                Button(L("描き続ける"), role: .cancel) {}
            }
        }
    }

    private var tools: some View {
        HStack(spacing: RenderTheme.Spacing.md) {
            // Mac はまだペンだけ(消しゴム・元に戻すは #1342)
            #if os(iOS)
            Picker(L("道具"), selection: Binding(get: { session.tool }, set: { session.tool = $0 })) {
                Label(L("ペン"), systemImage: "pencil.tip").tag(DrawingSession.Tool.pen)
                Label(L("消しゴム"), systemImage: "eraser").tag(DrawingSession.Tool.eraser)
            }
            .pickerStyle(.segmented)
            .frame(maxWidth: 160)
            #endif
            Spacer(minLength: 0)
            #if os(iOS)
            Button { canvas.undo() } label: { Image(systemName: "arrow.uturn.backward") }
                .disabled(!canvas.canUndo)
                .accessibilityLabel(L("元に戻す"))
            Button { canvas.redo() } label: { Image(systemName: "arrow.uturn.forward") }
                .disabled(!canvas.canRedo)
                .accessibilityLabel(L("やり直す"))
            #endif
            Button(role: .destructive) { confirmClear = true } label: { Image(systemName: "trash") }
                .disabled(session.isEmpty)
                .accessibilityLabel(L("全部消す"))
                .accessibilityHint(L("描いた線と前に描いた絵を全部消します"))
        }
        .font(.title3)
        .frame(minHeight: 44)
        .padding(.horizontal, RenderTheme.Spacing.md)
        .padding(.vertical, RenderTheme.Spacing.sm)
    }
}

/// キャンバスへの操作(元に戻す・やり直す)と、その押せる / 押せないの状態。
@MainActor
@Observable
final class CanvasHandle {
    #if os(iOS)
    @ObservationIgnored fileprivate weak var view: IsolatedCanvasView?
    #else
    @ObservationIgnored fileprivate weak var view: NSView?
    #endif
    private(set) var canUndo = false
    private(set) var canRedo = false

    func undo() { view?.undoManager?.undo(); refresh() }
    func redo() { view?.undoManager?.redo(); refresh() }

    /// 全部消した後は前の線に戻せない(戻すと消えた下絵と食い違う)
    func didClear() {
        view?.undoManager?.removeAllActions()
        refresh()
    }

    fileprivate func refresh() {
        canUndo = view?.undoManager?.canUndo ?? false
        canRedo = view?.undoManager?.canRedo ?? false
    }
}

#if os(iOS)
/// フォームの入力欄の「取り消し」と混ざらないよう、キャンバスだけの UndoManager を持つ。
fileprivate final class IsolatedCanvasView: PKCanvasView {
    private let ownUndo = UndoManager()
    override var undoManager: UndoManager? { ownUndo }
}

private struct DrawingCanvas: UIViewRepresentable {
    let session: DrawingSession
    let handle: CanvasHandle

    func makeCoordinator() -> Coordinator { Coordinator(session: session, handle: handle) }

    func makeUIView(context: Context) -> IsolatedCanvasView {
        let v = IsolatedCanvasView()
        v.drawingPolicy = .anyInput // 指でも Apple Pencil でも描く
        v.backgroundColor = .clear
        v.isOpaque = false
        v.overrideUserInterfaceStyle = .light // 保存する絵と同じ白地に黒で見せる
        v.alwaysBounceVertical = false
        v.drawing = session.drawing
        v.tool = Self.tool(session.tool)
        v.delegate = context.coordinator
        v.isAccessibilityElement = true
        v.accessibilityLabel = L("手書きのキャンバス")
        v.accessibilityHint = L("指や Apple Pencil で描きます")
        v.accessibilityTraits = .allowsDirectInteraction
        handle.view = v
        context.coordinator.revision = session.revision
        Self.updateValue(v, session: session)
        return v
    }

    func updateUIView(_ v: IsolatedCanvasView, context: Context) {
        v.tool = Self.tool(session.tool)
        // 全部消した時だけキャンバスに戻す(描いている線はキャンバスが正)
        if context.coordinator.revision != session.revision {
            context.coordinator.revision = session.revision
            v.drawing = session.drawing
        }
        Self.updateValue(v, session: session)
    }

    @MainActor static func tool(_ t: DrawingSession.Tool) -> PKTool {
        switch t {
        case .pen: PKInkingTool(.pen, color: .black, width: 6)
        case .eraser: PKEraserTool(.vector)
        }
    }

    @MainActor static func updateValue(_ v: UIView, session: DrawingSession) {
        v.accessibilityValue = session.accessibilityValue
    }

    @MainActor
    final class Coordinator: NSObject, PKCanvasViewDelegate {
        let session: DrawingSession
        let handle: CanvasHandle
        var revision = 0

        init(session: DrawingSession, handle: CanvasHandle) {
            self.session = session
            self.handle = handle
        }

        func canvasViewDrawingDidChange(_ canvasView: PKCanvasView) {
            session.drawing = canvasView.drawing
            handle.refresh()
            DrawingCanvas.updateValue(canvasView, session: session)
        }
    }
}
#endif

#if os(macOS)
/// Mac のキャンバス: マウス・トラックパッドの線を集めて `DrawingSession.drawing` に足す(ペンだけ)。
private struct MacDrawingCanvas: NSViewRepresentable {
    let session: DrawingSession

    func makeCoordinator() -> Coordinator { Coordinator() }

    func makeNSView(context: Context) -> MacDrawingView {
        let v = MacDrawingView()
        v.setAccessibilityElement(true)
        v.setAccessibilityRole(.image)
        v.setAccessibilityLabel(L("手書きのキャンバス"))
        v.setAccessibilityHelp(L("マウスやトラックパッドで描きます"))
        v.onStroke = { [session, weak v] stroke in
            session.add(stroke)
            v?.drawing = session.drawing
            v?.setAccessibilityValue(session.accessibilityValue)
        }
        v.drawing = session.drawing
        v.setAccessibilityValue(session.accessibilityValue)
        context.coordinator.revision = session.revision
        return v
    }

    func updateNSView(_ v: MacDrawingView, context: Context) {
        // 全部消した時だけキャンバスに戻す
        if context.coordinator.revision != session.revision {
            context.coordinator.revision = session.revision
            v.drawing = session.drawing
        }
        v.setAccessibilityValue(session.accessibilityValue)
    }

    final class Coordinator { var revision = 0 }
}

/// 描いた線(`PKDrawing` を画像にして)+ 描いている途中の線を出す。原点は左上(PKDrawing と同じ向き)。
final class MacDrawingView: NSView {
    var onStroke: ((PKStroke) -> Void)?
    var drawing = PKDrawing() {
        didSet { inkImage = nil; needsDisplay = true }
    }
    private var builder: DrawingStrokeBuilder?
    private var inkImage: NSImage?

    override var isFlipped: Bool { true }
    override func acceptsFirstMouse(for event: NSEvent?) -> Bool { true }

    override func setFrameSize(_ newSize: NSSize) {
        super.setFrameSize(newSize)
        inkImage = nil
    }

    override func mouseDown(with event: NSEvent) {
        builder = DrawingStrokeBuilder()
        record(event)
    }

    override func mouseDragged(with event: NSEvent) { record(event) }

    override func mouseUp(with event: NSEvent) {
        record(event)
        let stroke = builder?.stroke()
        builder = nil
        needsDisplay = true
        if let stroke { onStroke?(stroke) }
    }

    private func record(_ event: NSEvent) {
        guard builder != nil else { return }
        let p = convert(event.locationInWindow, from: nil)
        // 筆圧はペンタブレットの時だけ(マウスは押している間ずっと 1)
        let pressure: CGFloat? = event.subtype == .tabletPoint ? CGFloat(event.pressure) : nil
        builder?.add(p, time: event.timestamp, pressure: pressure)
        needsDisplay = true
    }

    override func draw(_ dirtyRect: NSRect) {
        if !drawing.strokes.isEmpty {
            if inkImage == nil {
                // 保存する絵と同じく白地に黒(ダークモードでインクを反転させない)
                let scale = window?.backingScaleFactor ?? 2
                (NSAppearance(named: .aqua) ?? NSAppearance.currentDrawing()).performAsCurrentDrawingAppearance {
                    inkImage = drawing.image(from: bounds, scale: scale)
                }
            }
            inkImage?.drawUpright(in: bounds)
        }
        guard let builder, let first = builder.locations.first else { return }
        NSColor.black.set()
        let width = builder.points.last.map { $0.size.width } ?? DrawingStrokeBuilder.penWidth
        if builder.locations.count == 1 {
            NSBezierPath(ovalIn: CGRect(x: first.x - width / 2, y: first.y - width / 2, width: width, height: width)).fill()
            return
        }
        let path = NSBezierPath()
        path.lineWidth = width
        path.lineCapStyle = .round
        path.lineJoinStyle = .round
        path.move(to: first)
        for p in builder.locations.dropFirst() { path.line(to: p) }
        path.stroke()
    }
}
#endif
