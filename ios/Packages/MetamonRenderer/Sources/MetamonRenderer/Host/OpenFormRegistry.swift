import SwiftUI

/// 今描いているフォームの控え(M10 の Mac 版・#943)。Esc・⌘W・アプリの切り替えのように
/// `ScreenView` の外から入力を失わせる操作の前に、保存していない入力があるかを確かめる。
/// ホストが `\.openForms` に入れた時だけ `ScreenView` が登録する(iOS は入れない)。
/// フォームは弱く持つ(画面が消えたら控えからも消える)。
@MainActor
public final class OpenFormRegistry {
    private struct Entry {
        weak var form: FormDraft?
        let route: Route
    }

    private var entries: [ObjectIdentifier: Entry] = [:]

    public init() {}

    public func track(_ form: FormDraft, at route: Route) {
        entries[ObjectIdentifier(form)] = Entry(form: form, route: route)
    }

    public func untrack(_ form: FormDraft) {
        entries[ObjectIdentifier(form)] = nil
    }

    /// 保存していない入力のあるフォームの画面のうち、`shown` が今も出ているとみなすもの。
    public func unsavedRoutes(where shown: (Route) -> Bool) -> [Route] {
        entries = entries.filter { $0.value.form != nil }
        return entries.values.compactMap { entry in
            guard let form = entry.form, form.hasUnsavedChanges, shown(entry.route) else { return nil }
            return entry.route
        }
    }
}

private struct OpenFormRegistryKey: EnvironmentKey {
    static let defaultValue: OpenFormRegistry? = nil
}

public extension EnvironmentValues {
    /// フォームを登録する先(Mac のランタイムが入れる)。nil なら登録しない。
    var openForms: OpenFormRegistry? {
        get { self[OpenFormRegistryKey.self] }
        set { self[OpenFormRegistryKey.self] = newValue }
    }
}

public extension View {
    /// 保存していない入力を破棄するかの確認(M10)。iOS でフォームを閉じる時と、Mac の Esc・⌘W・アプリの切り替えで同じ文言。
    func discardFormConfirmation(isPresented: Binding<Bool>, onDiscard: @escaping () -> Void) -> some View {
        confirmationDialog(L("編集内容を破棄しますか?"), isPresented: isPresented, titleVisibility: .visible) {
            Button(L("破棄"), role: .destructive, action: onDiscard)
            Button(L("編集を続ける"), role: .cancel) {}
        } message: {
            Text(L("保存していない入力は失われます。"))
        }
    }
}
