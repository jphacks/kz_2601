import Foundation
import MetamonDSL

/// #253 のモック: 「予約」で、お客さんの名前・電話番号が誰でも見られる(public)予約アプリと `meta.publicPersonalData` を返す。
/// 確認ダイアログの「この欄を消す」/「自分だけにする」の指示文にも応える(UI テスト・スクショ用)。
enum MockPublicData {
    static func matches(_ transcript: String) -> Bool { transcript.contains("予約") }

    static let fields = [
        PublicPersonalField(entity: "reservations", entityName: "予約", field: "guestName", fieldName: "お名前", kind: "name"),
        PublicPersonalField(entity: "reservations", entityName: "予約", field: "phone", fieldName: "電話番号", kind: "phone"),
    ]

    static func morph(transcript: String, appId: String, version: Int) throws -> MorphResult {
        let makePrivate = transcript.contains("自分だけ") || transcript.contains("only me")
        let removeFields = transcript.contains("消して") || transcript.hasPrefix("Remove")
        let json = dsl(scope: makePrivate ? nil : "public", withPersonalFields: !removeFields)
        let doc = AppDocument(appId: appId, version: version, dsl: try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8)))
        if makePrivate { return MorphResult(document: doc, summary: "予約を自分だけの記録にしました(モック)", mode: "patch") }
        if removeFields { return MorphResult(document: doc, summary: "「お名前」「電話番号」の欄を消しました(モック)", mode: "patch") }
        let summary = "「美容室の予約」を用意しました。予約は、このアプリを開いた人なら誰でも見られます(みんなに公開)。"
            + "「お名前」「電話番号」のような個人の情報も公開されます。ほかの人に見られてよいか確かめてください。(モック)"
        return MorphResult(document: doc, summary: summary, mode: "full", publicPersonalData: fields)
    }

    /// チャット: 依頼に電話番号があり、まだ確かめていなければ 1 回だけ `confirmPublic` の question。
    static func confirmTurn(messages: [ChatMessage], turn: Int, mode: ChatMode) -> ChatTurn? {
        let asked = messages.contains { $0.role == .assistant && $0.text.contains("公開してよいですか") }
        guard !asked, messages.contains(where: { $0.role == .user && $0.text.contains("電話") }) else { return nil }
        return ChatTurn(kind: .question,
                        text: "名前・電話番号のような個人の情報が、アプリを開いた人なら誰でも見られる記録に入ります。公開してよいですか?",
                        suggestions: ["公開してよい", "個人の情報は入れない", "自分だけの記録にする"],
                        turn: turn, locale: "ja", source: "mock", mode: mode.rawValue, confirmPublic: ["name", "phone"])
    }

    private static func dsl(scope: String?, withPersonalFields: Bool) -> String {
        let scopeJSON = scope.map { #""scope": "\#($0)","# } ?? ""
        let personal = withPersonalFields ? #"""
                { "id": "guestName", "name": "お名前", "type": "string", "required": true },
                { "id": "phone", "name": "電話番号", "type": "string" },
        """# : ""
        let personalInputs = withPersonalFields ? #"""
                { "type": "textField", "field": "guestName", "label": "お名前", "placeholder": "山田 花子" },
                { "type": "textField", "field": "phone", "label": "電話番号", "placeholder": "090-0000-0000" },
        """# : ""
        let title = withPersonalFields ? "guestName" : "menu"
        return #"""
        {
          "schemaVersion": 1,
          "app": { "name": "美容室の予約", "icon": "scissors", "voiceEntry": "floating" },
          "initialScreen": "home",
          "entities": [
            {
              "id": "reservations", "name": "予約", \#(scopeJSON) "titleField": "\#(title)",
              "fields": [
        \#(personal)
                { "id": "menu", "name": "メニュー", "type": "enum", "options": ["カット", "カラー", "パーマ"], "defaultValue": "カット" },
                { "id": "at", "name": "日時", "type": "datetime", "defaultValue": "now" }
              ]
            }
          ],
          "screens": [
            {
              "id": "home", "title": "予約",
              "toolbar": [{ "label": "追加", "icon": "plus", "action": { "type": "navigate", "screen": "reserve_form", "presentation": "sheet" } }],
              "components": [
                { "type": "section", "id": "list", "area": "A1:F10", "title": "予約一覧",
                  "children": [
                    { "type": "list", "entity": "reservations", "item": { "title": "\#(title)", "subtitle": "menu", "trailing": "at" },
                      "sort": { "field": "at", "direction": "asc" }, "emptyText": "まだ予約はありません",
                      "onItemTap": { "type": "navigate", "screen": "reserve_form", "presentation": "push" } }
                  ] },
                { "type": "button", "id": "add", "area": "D11:F12", "label": "予約を追加", "icon": "plus",
                  "action": { "type": "navigate", "screen": "reserve_form", "presentation": "sheet" } }
              ]
            },
            {
              "id": "reserve_form", "title": "予約", "entity": "reservations",
              "toolbar": [{ "label": "保存", "icon": "checkmark", "action": { "type": "saveRecord", "entity": "reservations" } }],
              "components": [
                { "type": "section", "id": "fields", "area": "A1:F8", "title": "内容",
                  "children": [
        \#(personalInputs)
                    { "type": "picker", "field": "menu", "label": "メニュー", "style": "segmented" },
                    { "type": "datePicker", "field": "at", "label": "日時" }
                  ] }
              ]
            }
          ]
        }
        """#
    }
}
