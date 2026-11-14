import SwiftUI

/// 編集(2 回目以降の変身)で変わった部品(#626)。ホストが apply の直後に渡し、Renderer は画面の出現時に
/// 変わった部品だけを光らせて置き直す。変わっていない部品は待たせずにすぐ出す(変わった所が目立つように)。
public struct ChangeHighlight: Equatable, Sendable {
    /// `key(screen:component:)` の集合
    public var components: Set<String>
    /// 変わった部品を持つ画面 id。ここに無い画面はいつもどおりの出現
    public var screens: Set<String>

    public init(components: Set<String> = [], screens: Set<String> = []) {
        self.components = components
        self.screens = screens
    }

    public static let none = ChangeHighlight()
    public var isEmpty: Bool { components.isEmpty }

    public static func key(screen: String, component: String) -> String { screen + "/" + component }
}

private struct ChangeHighlightKey: EnvironmentKey {
    static let defaultValue = ChangeHighlight.none
}

public extension EnvironmentValues {
    var changeHighlight: ChangeHighlight {
        get { self[ChangeHighlightKey.self] }
        set { self[ChangeHighlightKey.self] = newValue }
    }
}
