import SwiftUI
import AppKit

/// メニューバー(#934)。ウインドウの状態は `focusedSceneValue` で受け取り、今の画面に合わせて有効 / 無効を変える。
/// - ファイル: 新しいアプリ・文字で頼む(会話のシート・#967)・声で頼む(同じシートですぐ録音・#966)・新しいウインドウ・
///   保存 ⌘S(#1138)・CSV / PDF で書き出す・プリント ⌘P(#1139・今の画面に一覧がある時だけ)
/// - 編集: 元に戻す(記録を消した直後は削除の取り消し・それ以外は入力欄の取り消し)・やり直す・アプリを探す(⌘F・#1282)
/// - アプリ: 生成したアプリの版の「変更を元に戻す」(⌥⌘Z)・「変更をやり直す」(⇧⌥⌘Z)(#968)・次の通知(#1325)。
///   ⌘Z / ⇧⌘Z は入力欄と記録の削除に使っているので、版の取り消しは別のキーにする
/// - 表示: サイドバー(`SidebarCommands`)・ツールバー(`ToolbarCommands`)・画面マップ(⇧⌘E・#970)・
///   拡大 ⌘+ / 縮小 ⌘- / 実際のサイズ ⌘0(中央のアプリの文字・#1206)
/// - 移動: 戻る・アプリの一覧 / 中央へ・前 / 次のアプリ
/// - ヘルプ: 頼み方のコツ(#967)・お問い合わせ・フィードバックを送る(#983)
struct MacCommands: Commands {
    static let mainWindowId = "main"
    static let contactURL = URL(string: "https://tailor-it.app/contact")!

    @FocusedValue(\.shellActions) private var shell
    @FocusedValue(\.macAppRuntime) private var runtime
    @Environment(\.openWindow) private var openWindow
    @Environment(\.openURL) private var openURL
    /// 中央のアプリの文字の段(#1206)。`AppCanvas` と同じ値を読む
    @AppStorage(MacTextZoom.storageKey) private var zoomStep = MacTextZoom.defaultStep

    /// 会話のシート・画面マップが開いている間は中央のアプリに向けた操作(保存・書き出し)を押せない
    private var canUseCanvas: Bool { shell?.canCompose ?? true }

    var body: some Commands {
        SidebarCommands()
        ToolbarCommands()

        CommandGroup(replacing: .newItem) {
            Button("新しいアプリ") { shell?.newApp() }
                .keyboardShortcut("n", modifiers: .command)
                .disabled(!(shell?.canCompose ?? false))
            Button("新しいウインドウ") { openWindow(id: Self.mainWindowId) }
                .keyboardShortcut("n", modifiers: [.command, .option])
            Divider()
            // シートが開いている間は押せない(⇧⌘M はシートのマイクが受けて、録音の開始 / 終了になる)
            Button("声で頼む") { shell?.speak() }
                .keyboardShortcut("m", modifiers: [.command, .shift])
                .disabled(!(shell?.canCompose ?? false))
            Button("文字で頼む") { shell?.ask() }
                .keyboardShortcut("k", modifiers: .command)
                .disabled(!(shell?.canCompose ?? false))
        }

        saveAndExportCommands

        CommandGroup(replacing: .undoRedo) {
            if let runtime, runtime.recordUndo != nil {
                Button("削除を元に戻す") { Task { await runtime.undoRecordDelete() } }
                    .keyboardShortcut("z", modifiers: .command)
            } else {
                // 入力欄の取り消しはレスポンダチェーン(フィールドエディタ・ウインドウの undoManager)に任せる
                Button("元に戻す") { NSApp.sendAction(Selector(("undo:")), to: nil, from: nil) }
                    .keyboardShortcut("z", modifiers: .command)
            }
            Button("やり直す") { NSApp.sendAction(Selector(("redo:")), to: nil, from: nil) }
                .keyboardShortcut("z", modifiers: [.command, .shift])
        }

        // サイドバーの検索欄に入る(#1282)
        CommandGroup(after: .pasteboard) {
            Divider()
            Button("アプリを探す") { shell?.focusSearch() }
                .keyboardShortcut("f", modifiers: .command)
                .disabled(shell == nil)
        }

        CommandGroup(after: .toolbar) {
            Divider()
            // 中央のアプリのシート(sheet / fullScreen)が開いている間は出さない(シートを重ねない)
            Button("画面マップ") { shell?.showScreenMap() }
                .keyboardShortcut("e", modifiers: [.command, .shift])
                .disabled(!(shell?.canShowScreenMap ?? false) || runtime?.sheet != nil || runtime?.fullScreen != nil
                           || runtime?.calendarDraft != nil)
            Divider()
            // 中央のアプリの文字の大きさ(#1206)。端の段と、画面マップ・会話のシートの間は押せない(マップは自分の拡大縮小に同じキーを使う)
            Button("拡大") { zoomStep = MacTextZoom.zoomedIn(zoomStep) }
                .keyboardShortcut("+", modifiers: .command)
                .disabled(!canUseCanvas || !MacTextZoom.canZoomIn(zoomStep))
            Button("縮小") { zoomStep = MacTextZoom.zoomedOut(zoomStep) }
                .keyboardShortcut("-", modifiers: .command)
                .disabled(!canUseCanvas || !MacTextZoom.canZoomOut(zoomStep))
            Button("実際のサイズ") { zoomStep = MacTextZoom.defaultStep }
                .keyboardShortcut("0", modifiers: .command)
                .disabled(!canUseCanvas || MacTextZoom.isActualSize(zoomStep))
        }

        CommandMenu("アプリ") {
            Button("変更を元に戻す") { shell?.undoVersion() }
                .keyboardShortcut("z", modifiers: [.command, .option])
                .disabled(!(shell?.canUndoVersion ?? false))
            Button("変更をやり直す") { shell?.redoVersion() }
                .keyboardShortcut("z", modifiers: [.command, .option, .shift])
                .disabled(!(shell?.canRedoVersion ?? false))
            Divider()
            // ツールバーのベルと同じポップオーバー(#1325)。中央のアプリのシートの間は出さない
            Button("次の通知") { runtime?.notifications.showsUpcoming = true }
                .disabled(runtime == nil || runtime?.sheet != nil || runtime?.fullScreen != nil)
        }

        CommandMenu("移動") {
            Button("戻る") { runtime?.requestGoBack() }
                .keyboardShortcut("[", modifiers: .command)
                .disabled(!(runtime?.canGoBack ?? false))
            Divider()
            Button("アプリの一覧へ") { shell?.focusSidebar() }
                .keyboardShortcut("1", modifiers: .command)
                .disabled(shell == nil)
            Button("選んだアプリへ") { shell?.focusCanvas() }
                .keyboardShortcut("2", modifiers: .command)
                .disabled(!(shell?.hasSelection ?? false))
            Divider()
            Button("前のアプリ") { shell?.selectPrevious() }
                .keyboardShortcut(.upArrow, modifiers: [.command, .option])
                .disabled(!(shell?.canSelectPrevious ?? false))
            Button("次のアプリ") { shell?.selectNext() }
                .keyboardShortcut(.downArrow, modifiers: [.command, .option])
                .disabled(!(shell?.canSelectNext ?? false))
        }

        CommandGroup(replacing: .help) {
            Button("頼み方のコツ") { shell?.showTips() }
                .disabled(!(shell?.canCompose ?? false))
            Button("お問い合わせ") { openURL(Self.contactURL) }
            #if !APP_STORE_REVIEW // 審査用ビルドはフィードバックを外す(#319)
            Button("フィードバックを送る…") { MacFeedbackCenter.shared.open(openWindow) } // #983
            #endif
        }
    }

    /// ファイルの保存・書き出し・プリント(`body` の項目数の上限を超えないよう分ける)
    @CommandsBuilder private var saveAndExportCommands: some Commands {
        // 中央のフォームを保存バーと同じ流れで保存する(#1138)。⌘Return は中央の列の隠しボタン(`AppCanvas`)
        CommandGroup(after: .saveItem) {
            Button("保存") { Task { await runtime?.saveFromMenu() } }
                .keyboardShortcut("s", modifiers: .command)
                .disabled(!canUseCanvas || !(runtime?.canSaveFromMenu ?? false))
        }

        // 今の画面の一覧(list / table)を export と同じ流れで出す(#1139)
        CommandGroup(replacing: .printItem) {
            Button("CSV で書き出す…") { Task { await runtime?.exportFromMenu(format: "csv") } }
                .disabled(!canUseCanvas || !(runtime?.canExportFromMenu ?? false))
            Button("PDF で書き出す…") { Task { await runtime?.exportFromMenu(format: "pdf") } }
                .disabled(!canUseCanvas || !(runtime?.canExportFromMenu ?? false))
            Divider()
            Button("プリント…") { Task { await runtime?.printFromMenu() } }
                .keyboardShortcut("p", modifiers: .command)
                .disabled(!canUseCanvas || !(runtime?.canExportFromMenu ?? false))
        }
    }
}

/// ウインドウ(MacRootView)がメニューに渡す操作と状態。
struct ShellActions {
    var hasSelection: Bool
    var canSelectPrevious: Bool
    var canSelectNext: Bool
    var selectPrevious: () -> Void
    var selectNext: () -> Void
    var focusSidebar: () -> Void
    var focusCanvas: () -> Void
    /// ⌘F: サイドバーの検索欄に入る(#1282)
    var focusSearch: () -> Void = {}
    /// 会話のシートを開けるか(開いている間は新しく開かない)
    var canCompose: Bool
    /// 中央のアプリの版を戻せる / やり直せるか(#968・会話のシートが開いている間は押せない)
    var canUndoVersion: Bool = false
    var canRedoVersion: Bool = false
    var undoVersion: () -> Void = {}
    var redoVersion: () -> Void = {}
    var newApp: () -> Void
    /// 文字で頼む(中央のアプリを直す・無ければ作る)
    var ask: () -> Void
    /// 声で頼む(⌘K と同じ相手の会話を開いてすぐ録音・#966)
    var speak: () -> Void
    var showTips: () -> Void
    /// 画面マップ(#970)を開けるか(中央にアプリがあり、会話のシートが閉じている)
    var canShowScreenMap: Bool = false
    var showScreenMap: () -> Void = {}
}

private struct ShellActionsKey: FocusedValueKey {
    typealias Value = ShellActions
}

private struct MacAppRuntimeKey: FocusedValueKey {
    typealias Value = MacAppRuntime
}

extension FocusedValues {
    var shellActions: ShellActions? {
        get { self[ShellActionsKey.self] }
        set { self[ShellActionsKey.self] = newValue }
    }

    /// 中央で開いているアプリのランタイム(戻る・削除の取り消し)
    var macAppRuntime: MacAppRuntime? {
        get { self[MacAppRuntimeKey.self] }
        set { self[MacAppRuntimeKey.self] = newValue }
    }
}
