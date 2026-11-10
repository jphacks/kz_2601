import SwiftUI
import AppKit

/// メニューバー(#934)。ウインドウの状態は `focusedSceneValue` で受け取り、今の画面に合わせて有効 / 無効を変える。
/// - ファイル: 新しいアプリ・文字で頼む(会話のシート・#967)・声で頼む(#966 まで押せない)・新しいウインドウ
/// - 編集: 元に戻す(記録を消した直後は削除の取り消し・それ以外は入力欄の取り消し)・やり直す
/// - 表示: サイドバー(`SidebarCommands`)・ツールバー(`ToolbarCommands`)
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
            Button("声で頼む") {}
                .keyboardShortcut("m", modifiers: [.command, .shift])
                .disabled(true)
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
    var newApp: () -> Void
    /// 文字で頼む(中央のアプリを直す・無ければ作る)
    var ask: () -> Void
    var showTips: () -> Void
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
