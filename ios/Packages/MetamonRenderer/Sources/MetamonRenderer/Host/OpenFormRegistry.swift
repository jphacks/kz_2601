import SwiftUI
import Observation

/// 今描いているフォームの控え(M10 の Mac 版・#943)。Esc・⌘W・アプリの切り替えのように
/// `ScreenView` の外から入力を失わせる操作の前に、保存していない入力があるかを確かめる。
/// ホストが `\.openForms` に入れた時だけ `ScreenView` が登録する(iOS は入れない)。
/// フォームは弱く持つ(画面が消えたら控えからも消える)。
/// メニューの「保存」(#1138)が押せるかを追えるよう、登録が変わるたびに `revision` を進める(Observation)。
@MainActor
@Observable
public final class OpenFormRegistry {
    private struct Entry {
        weak var form: FormDraft?
        let route: Route
        let order: Int
    }

    @ObservationIgnored private var entries: [ObjectIdentifier: Entry] = [:]
    /// 登録・解除の回数。読んだ側は登録が変わると描き直される
    public private(set) var revision = 0

    public init() {}

    public func track(_ form: FormDraft, at route: Route) {
        revision += 1
        entries[ObjectIdentifier(form)] = Entry(form: form, route: route, order: revision)
    }

    public func untrack(_ form: FormDraft) {
        guard entries.removeValue(forKey: ObjectIdentifier(form)) != nil else { return }
        revision += 1
    }

    /// `route` の画面で描いているフォーム(同じ画面を開き直したら、あとから登録した方)。
    public func form(at route: Route) -> FormDraft? {
        _ = revision
        return entries.values.filter { $0.route == route && $0.form != nil }.max { $0.order < $1.order }?.form
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
