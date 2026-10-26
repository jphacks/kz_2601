import SwiftUI
import MetamonDSL

/// 「このデータは誰でも見られます(公開)」の控えめな表示(#253)。DSL の `entity.scope` を読むだけ。
/// public / shared の entity の一覧の上と、その entity のフォームの上に出す。private(省略時)は何も出さない。
struct PublicDataNotice: View {
    let scope: EntityScope

    var body: some View {
        if let text = Self.text(for: scope) {
            Label {
                Text(text)
            } icon: {
                Image(systemName: scope == .public ? "globe" : "person.2")
            }
            .font(.caption)
            .foregroundStyle(.secondary)
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("publicDataNotice")
        }
    }

    /// 表示する文言(private は nil)。
    static func text(for scope: EntityScope) -> String? {
        switch scope {
        case .public: L("このデータは誰でも見られます(公開)")
        case .shared: L("このデータは参加者だけが見られます(共有)")
        case .private: nil
        }
    }
}

private struct PublicListNotice: ViewModifier {
    @Environment(\.dslRuntime) private var rt
    let entity: String?

    func body(content: Content) -> some View {
        let scope = entity.map { rt.dsl.scope(of: $0) } ?? .private
        if scope.isVisibleToOthers {
            VStack(alignment: .leading, spacing: RenderTheme.Spacing.xs) {
                PublicDataNotice(scope: scope).padding(.horizontal, RenderTheme.Spacing.xs)
                content
            }
        } else {
            content
        }
    }
}

/// フォーム画面(`screen.entity` あり)の上端に出す。
struct PublicFormNotice: ViewModifier {
    @Environment(\.dslRuntime) private var rt
    let screen: Screen

    func body(content: Content) -> some View {
        let scope = screen.entity.map { rt.dsl.scope(of: $0) } ?? .private
        content.safeAreaInset(edge: .top, spacing: 0) {
            if scope.isVisibleToOthers {
                PublicDataNotice(scope: scope)
                    .padding(.horizontal, RenderTheme.Spacing.screenInset)
                    .padding(.vertical, RenderTheme.Spacing.xs + 2)
                    .background(RenderTheme.Colors.canvas)
            }
        }
    }
}

extension View {
    /// 一覧の上に公開の表示を足す(entity が public / shared の時だけ)。
    func publicDataNotice(entity: String?) -> some View { modifier(PublicListNotice(entity: entity)) }
}
