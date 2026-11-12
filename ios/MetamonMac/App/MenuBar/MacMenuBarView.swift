import AppKit
import SwiftUI

/// メニューバーの窓(#1848・`MenuBarExtra` の window の形と同じ見た目)。選んだ生成アプリの stat と「開く」「頼む」。
/// 生成アプリのボタン(`exposeAsControl`)は #1775 の決まり待ちなので、まだ出さない
struct MacMenuBarView: View {
    @State private var model = MacMenuBarModel()
    /// 窓を閉じて本体のウインドウを前に出す(`MacStatusItem`)
    let bringMainWindow: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if model.apps.isEmpty {
                Text(model.isLoading ? "読み込み中…" : "作ったアプリがありません")
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
            } else {
                Picker("アプリ", selection: Binding(get: { model.selectedId ?? "" },
                                                 set: { id in Task { await model.select(id) } })) {
                    ForEach(model.apps) { app in Text(verbatim: app.name).tag(app.id) }
                }
                .labelsHidden()
                .accessibilityIdentifier("menuBar.app")
                stats
            }
            Divider()
            HStack(spacing: 8) {
                Button { open() } label: { Label("アプリを開く", systemImage: "macwindow") }
                    .disabled(model.selectedId == nil)
                    .accessibilityIdentifier("menuBar.open")
                Button { talk() } label: { Label("声・文字で頼む", systemImage: "mic") }
                    .accessibilityIdentifier("menuBar.talk")
            }
        }
        .padding(14)
        .frame(width: 300)
        .task { await model.reload() }
        .onReceive(NotificationCenter.default.publisher(for: IntentHost.recordAdded)) { note in
            Task { await model.recordsChanged(appId: note.object as? String) }
        }
    }

    @ViewBuilder private var stats: some View {
        if model.stats.isEmpty {
            Text(model.isLoading ? "読み込み中…" : "このアプリに集計(stat)の部品がありません")
                .font(.callout)
                .foregroundStyle(.secondary)
        } else {
            VStack(alignment: .leading, spacing: 8) {
                ForEach(model.stats) { stat in
                    LabeledContent {
                        Text(verbatim: stat.value).font(.title3.monospacedDigit().weight(.semibold))
                    } label: {
                        Text(verbatim: stat.label)
                    }
                    .accessibilityElement(children: .combine)
                }
            }
        }
    }

    /// ショートカットの「開く」と同じ経路(`IntentHost.pending`)。ウインドウが無ければ作る
    private func open() {
        guard let appId = model.selectedId else { return }
        Task { await IntentHost.shared.request(.open(appId: appId)) }
        bringMainWindow()
    }

    /// ショートカットの「話しかける」と同じ(⇧⌘M の会話。声でも文字でも頼める)
    private func talk() {
        Task { await IntentHost.shared.request(.talk) }
        bringMainWindow()
    }
}

/// 設定(一般)の「メニューバー」
struct MacMenuBarSettingsSection: View {
    @AppStorage(MacMenuBarSettings.enabledKey) private var enabled = false
    @State private var loginItem = MacLoginItem.shared

    var body: some View {
        Section {
            Toggle("メニューバーに表示する", isOn: $enabled)
                .accessibilityIdentifier("settings.menuBar")
                .onChange(of: enabled) { _, on in MacStatusItem.shared.apply(enabled: on) }
            Toggle("ログイン時に開く", isOn: Binding(get: { loginItem.isEnabled || loginItem.needsApproval },
                                                set: { loginItem.setEnabled($0) }))
                .accessibilityIdentifier("settings.openAtLogin")
            if loginItem.needsApproval {
                Text("システム設定の「ログイン項目」で許可してください。")
                    .font(.callout)
                    .foregroundStyle(.secondary)
            } else if loginItem.failed {
                Text("ログイン項目に登録できませんでした。")
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }
        } header: {
            Text("メニューバー")
        } footer: {
            Text("メニューバーのアイコンから、選んだアプリの集計を見て、アプリを開いたり頼んだりできます。")
        }
        .onAppear { loginItem.refresh() }
    }
}
