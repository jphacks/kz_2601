import SwiftUI
import MetamonDSL

/// schema の `type` 文字列 → View ビルダーの辞書。`DSLView` はここを引くだけで、switch を持たない。
///
/// **コンポーネントの追加手順(3 行)**
/// 1. `Components/Leaves/FooLeaf.swift`(or `Containers/`)に `struct FooLeaf: View` を1ファイル作る
/// 2. `MetamonDSL` の `ComponentType` に `case foo` を足す(schema `docs/dsl/dsl.schema.json` にも追加)
/// 3. 下の `standard` に `r.register(.foo) { c, ctx in FooLeaf(c: c, ctx: ctx) }` を1行足す
///    → `MetamonRendererTests` が schema の全 type と登録の一致を検証する(抜けたら赤)
public struct ComponentRegistry: Sendable {
    public typealias Builder = @MainActor @Sendable (Component, RenderContext) -> AnyView

    private var builders: [String: Builder] = [:]

    public init() {}

    /// `type` 文字列で登録(ホストが schema 外の独自コンポーネントを足す時もこちら)。
    public mutating func register<V: View>(_ type: String, @ViewBuilder _ build: @escaping @MainActor @Sendable (Component, RenderContext) -> V) {
        builders[type] = { AnyView(build($0, $1)) }
    }

    public mutating func register<V: View>(_ type: ComponentType, @ViewBuilder _ build: @escaping @MainActor @Sendable (Component, RenderContext) -> V) {
        register(type.rawValue, build)
    }

    /// 既存のレジストリに1件足した(or 上書きした)コピーを返す(アプリ本体での `voiceInput` 差し替えに使う)。
    public func registering<V: View>(_ type: ComponentType, @ViewBuilder _ build: @escaping @MainActor @Sendable (Component, RenderContext) -> V) -> ComponentRegistry {
        var copy = self
        copy.register(type, build)
        return copy
    }

    public func builder(for type: String) -> Builder? { builders[type] }

    public var registeredTypes: Set<String> { Set(builders.keys) }

    /// Renderer 同梱の全コンポーネント。
    /// `voiceInput` だけは音声機能(Services)に依存するため **アプリ本体が `registering(.voiceInput)` で差し替える**(ここではプレースホルダ)。
    public static let standard: ComponentRegistry = {
        var r = ComponentRegistry()
        // container
        r.register(.vstack) { c, ctx in VStackContainer(c: c, ctx: ctx) }
        r.register(.hstack) { c, ctx in HStackContainer(c: c, ctx: ctx) }
        r.register(.section) { c, ctx in SectionContainer(c: c, ctx: ctx) }
        // leaf
        r.register(.text) { c, _ in TextLeaf(c: c) }
        r.register(.button) { c, ctx in ButtonLeaf(c: c, ctx: ctx) }
        r.register(.textField) { c, ctx in TextFieldLeaf(c: c, ctx: ctx) }
        r.register(.numberField) { c, ctx in NumberFieldLeaf(c: c, ctx: ctx) }
        r.register(.datePicker) { c, ctx in DatePickerLeaf(c: c, ctx: ctx) }
        r.register(.picker) { c, ctx in PickerLeaf(c: c, ctx: ctx) }
        r.register(.toggle) { c, ctx in ToggleLeaf(c: c, ctx: ctx) }
        r.register(.photoField) { c, ctx in PhotoFieldLeaf(c: c, ctx: ctx) }
        r.register(.image) { c, ctx in ImageLeaf(c: c, ctx: ctx) }
        r.register(.list) { c, ctx in ListLeaf(c: c, ctx: ctx) }
        r.register(.stat) { c, _ in StatLeaf(c: c) }
        r.register(.chart) { c, _ in ChartLeaf(c: c) }
        r.register(.voiceInput) { c, _ in VoiceInputLeaf(c: c) }
        r.register(.spacer) { c, _ in SpacerLeaf(c: c) }
        r.register(.divider) { _, _ in DividerLeaf() }
        // container(構造系)
        r.register(.card) { c, ctx in CardContainer(c: c, ctx: ctx) }
        r.register(.disclosure) { c, ctx in DisclosureContainer(c: c, ctx: ctx) }
        r.register(.carousel) { c, ctx in CarouselContainer(c: c, ctx: ctx) }
        r.register(.grid) { c, ctx in GridContainer(c: c, ctx: ctx) }
        // leaf(ナビ・ネイティブ系)
        r.register(.searchBar) { c, ctx in SearchBarLeaf(c: c, ctx: ctx) }
        r.register(.calendar) { c, ctx in CalendarLeaf(c: c, ctx: ctx) }
        r.register(.shareButton) { c, ctx in ShareButtonLeaf(c: c, ctx: ctx) }
        r.register(.openUrlButton) { c, _ in OpenUrlButtonLeaf(c: c) }
        r.register(.mapView) { c, ctx in MapViewLeaf(c: c, ctx: ctx) }
        r.register(.locationButton) { c, ctx in LocationButtonLeaf(c: c, ctx: ctx) }
        r.register(.qrScanner) { c, ctx in QrScannerLeaf(c: c, ctx: ctx) }
        r.register(.timerDisplay) { c, ctx in TimerDisplayLeaf(c: c, ctx: ctx) }
        return r
    }()
}

private struct ComponentRegistryKey: EnvironmentKey {
    static let defaultValue: ComponentRegistry = .standard
}

public extension EnvironmentValues {
    /// 描画に使うレジストリ。既定は `.standard`。アプリ本体は `.environment(\.componentRegistry, ...)` で差し替える。
    var componentRegistry: ComponentRegistry {
        get { self[ComponentRegistryKey.self] }
        set { self[ComponentRegistryKey.self] = newValue }
    }
}
