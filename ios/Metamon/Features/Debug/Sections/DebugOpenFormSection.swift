#if METAMON_STG
import SwiftUI
import MetamonDSL
import MetamonServices

/// 開発用メニュー: 別の生成アプリのフォームを値入りで開く本体の口(#2235)を、DSL の openApp(#1893)なしで試す
final class DebugOpenFormSection: DebugMenuSection {
    override class var order: Int { 250 }
    override class var title: String { "別のアプリのフォームを開く" }
    override class var footer: String? {
        "選んだアプリの entity の入力画面を、値(項目の id か名前 → 値の JSON)を入れた新規で開きます。保存した時だけ記録が増え、保存すると今のアプリに戻ります。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugOpenFormRows()) }
}

private struct DebugOpenFormRows: View {
    @Environment(AppRuntime.self) private var rt
    @State private var appId = ""
    @State private var entityId = "expense"
    @State private var valuesJSON = #"{"品目":"牛乳","金額":"200"}"#
    @State private var message: String?

    var body: some View {
        Picker(selection: $appId) {
            Text(verbatim: "選ぶ").tag("")
            ForEach(rt.visibleApps, id: \.id) { app in Text(verbatim: app.name).tag(app.id) }
        } label: { Text(verbatim: "開くアプリ") }
        TextField(text: $entityId) { Text(verbatim: "entity の id") }
            .textInputAutocapitalization(.never)
            .autocorrectionDisabled()
        TextField(text: $valuesJSON) { Text(verbatim: "値の JSON") }
            .textInputAutocapitalization(.never)
            .autocorrectionDisabled()
            .font(.footnote.monospaced())
        Button {
            guard let data = valuesJSON.data(using: .utf8),
                  let values = try? JSONDecoder().decode([String: JSONValue].self, from: data) else {
                message = "値の JSON が読めません"
                return
            }
            Task {
                let outcome = await rt.openForm(appId: appId, entityId: entityId, values: values)
                message = "\(outcome)"
            }
        } label: {
            DebugKnobLabel(title: "値入りで開く", effect: .immediate)
        }
        .disabled(appId.isEmpty || entityId.isEmpty)
        .accessibilityIdentifier("debugMenu.openForm.open")
        if let message {
            Text(verbatim: message).font(.footnote).foregroundStyle(.secondary)
        }
    }
}
#endif
