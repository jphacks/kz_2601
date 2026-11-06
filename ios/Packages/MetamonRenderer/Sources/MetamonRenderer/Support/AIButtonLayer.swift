import SwiftUI
import MetamonDSL

/// 部品から開くシート(場所を選ぶ・曲を探す 等)で、AI ボタンの指示の対象にする部品(#629)。
public struct AIButtonComponent: Equatable, Sendable {
    public var id: String
    /// 表示名(label → title → 部品の種類)
    public var label: String

    public init(id: String, label: String) {
        self.id = id
        self.label = label
    }

    public init(_ c: Component) {
        let named = [c.label, c.title].compactMap { $0?.trimmingCharacters(in: .whitespacesAndNewlines) }.first { !$0.isEmpty }
        self.init(id: c.id, label: named ?? c.type.rawValue)
    }
}

/// ホストが部品のシートの中身を包む(ナビバーの AI ボタン・音声シートの表示元)。
public typealias AIButtonLayerWrapper = @MainActor (AnyView, AIButtonComponent) -> AnyView

private struct AIButtonLayerKey: EnvironmentKey {
    static let defaultValue: AIButtonLayerWrapper? = nil
}

private struct AIButtonComponentKey: EnvironmentKey {
    static let defaultValue: AIButtonComponent? = nil
}

public extension EnvironmentValues {
    /// 部品から開くシートに AI ボタンを足すホストの処理(#629)。nil なら何も足さない(プレビュー・テスト)。
    var aiButtonLayer: AIButtonLayerWrapper? {
        get { self[AIButtonLayerKey.self] }
        set { self[AIButtonLayerKey.self] = newValue }
    }

    /// 今のシートを開いた部品。部品の `.sheet` の中身に付ける。
    var aiButtonComponent: AIButtonComponent? {
        get { self[AIButtonComponentKey.self] }
        set { self[AIButtonComponentKey.self] = newValue }
    }
}

extension View {
    /// 部品から開くシートの中身に付ける: 指示の対象をこの部品にする。
    func aiButtonTarget(_ c: Component) -> some View {
        environment(\.aiButtonComponent, AIButtonComponent(c))
    }

    /// シートの中身(NavigationStack の内側)に付ける。ホストがナビバーに AI ボタンを置き、音声シートをここから出す。
    func aiButtonLayer() -> some View {
        modifier(AIButtonLayerModifier())
    }
}

private struct AIButtonLayerModifier: ViewModifier {
    @Environment(\.aiButtonLayer) private var wrap
    @Environment(\.aiButtonComponent) private var component

    func body(content: Content) -> some View {
        if let wrap, let component {
            wrap(AnyView(content), component)
        } else {
            content
        }
    }
}
