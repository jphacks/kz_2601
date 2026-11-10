import SwiftUI
import AppKit

/// メニューバー(#934)。ウインドウの状態は `focusedSceneValue` で受け取り、今の画面に合わせて有効 / 無効を変える。
/// - ファイル: 新しいアプリ・文字で頼む(会話のシート・#967)・声で頼む(同じシートですぐ録音・#966)・新しいウインドウ
/// - 編集: 元に戻す(記録を消した直後は削除の取り消し・それ以外は入力欄の取り消し)・やり直す
/// - アプリ: 生成したアプリの版の「変更を元に戻す」(⌥⌘Z)・「変更をやり直す」(⇧⌥⌘Z)(#968)。
///   ⌘Z / ⇧⌘Z は入力欄と記録の削除に使っているので、版の取り消しは別のキーにする
/// - 表示: サイドバー(`SidebarCommands`)・ツールバー(`ToolbarCommands`)・画面マップ(⇧⌘E・#970)
/// - 移動: 戻る・アプリの一覧 / 中央へ・前 / 次のアプリ
/// - ヘルプ: 頼み方のコツ(#967)・お問い合わせ
struct MacCommands: Commands {
    static let mainWindowId = "main"
    static let contactURL = URL(string: "https://tailor-it.app/contact")!

    @FocusedValue(\.shellActions) private var shell
    @FocusedValue(\.macAppRuntime) private var runtime
    @Environment(\.openWindow) private var openWindow
    @Environment(\.openURL) private var openURL

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

        CommandGroup(after: .toolbar) {
            Divider()
            // 中央のアプリのシート(sheet / fullScreen)が開いている間は出さない(シートを重ねない)
            Button("画面マップ") { shell?.showScreenMap() }
                .keyboardShortcut("e", modifiers: [.command, .shift])
                .disabled(!(shell?.canShowScreenMap ?? false) || runtime?.sheet != nil || runtime?.fullScreen != nil
                           || runtime?.calendarDraft != nil)
        }

        CommandMenu("アプリ") {
            Button("変更を元に戻す") { shell?.undoVersion() }
                .keyboardShortcut("z", modifiers: [.command, .option])
                .disabled(!(shell?.canUndoVersion ?? false))
            Button("変更をやり直す") { shell?.redoVersion() }
                .keyboardShortcut("z", modifiers: [.command, .option, .shift])
                .disabled(!(shell?.canRedoVersion ?? false))
        }

        CommandMenu("移動") {
            Button("戻る") { runtime?.goBack() }
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
