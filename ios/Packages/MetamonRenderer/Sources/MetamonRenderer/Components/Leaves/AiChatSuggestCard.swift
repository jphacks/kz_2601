// 審査用ビルドでは aiChat ごと外す(AiChatLeaf.swift と同じ)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL

/// `aiChat.saveTo` の記録の案: 最新の AI の返事の下に出すカード。
/// 保存は「追加」を押した時だけ(`createRecord` = フォームの保存と同じ経路。共有データの決まり・プランの上限・
/// 表示できない言葉は同期の列とサーバーがいつもどおり扱う)。「直す」はカードを閉じて入力欄に戻り、会話で言い直す。
struct AiChatSuggestion: Equatable {
    /// 案が付いた AI の発言(この吹き出しの下に出す)
    var turnId: String
    var entity: String
    /// 保存する data(案の値 + ほかの field の既定値)
    var data: [String: JSONValue]
    /// カードに出す行(field の名前・表示する値)
    var rows: [Row]
    var state: SaveState = .idle

    struct Row: Equatable {
        var name: String
        var value: String
    }

    enum SaveState: Equatable { case idle, saving, saved, failed(String) }

    /// 返事の案から作る。合う値が無い・required が埋まらない時は nil(カードを出さない)。
    static func make(turnId: String, suggest: [String: JSONValue]?, saveTo: AIChatSaveTo?, entity: Entity?,
                     display: (JSONValue, Field) -> String) -> AiChatSuggestion? {
        guard let suggest, let saveTo, let entity, let data = saveTo.record(suggest, in: entity) else { return nil }
        let rows = saveTo.values(suggest, in: entity).map { Row(name: $0.field.name, value: display($0.value, $0.field)) }
        return AiChatSuggestion(turnId: turnId, entity: entity.id, data: data, rows: rows)
    }
}

struct AiChatSuggestCard: View {
    let suggestion: AiChatSuggestion
    let onAdd: () -> Void
    let onFix: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            Text(L("この内容で記録しますか?")).font(.footnote.weight(.semibold))
            VStack(alignment: .leading, spacing: 2) {
                ForEach(Array(suggestion.rows.enumerated()), id: \.offset) { _, row in
                    HStack(alignment: .firstTextBaseline, spacing: RenderTheme.Spacing.sm) {
                        Text(row.name).font(.footnote).foregroundStyle(.secondary)
                        Text(row.value).font(.footnote).frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .accessibilityElement(children: .combine)
                }
            }
            footer
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RenderTheme.Colors.surfaceElevated, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay { RoundedRectangle(cornerRadius: 14, style: .continuous).strokeBorder(RenderTheme.Colors.hairline) }
        .padding(.trailing, 56)
        .accessibilityIdentifier("aiChat.suggest")
    }

    @ViewBuilder private var footer: some View {
        switch suggestion.state {
        case .saved:
            Label(L("記録しました"), systemImage: "checkmark.circle.fill")
                .font(.footnote).foregroundStyle(.secondary)
                .accessibilityIdentifier("aiChat.suggest.saved")
        case .idle, .saving, .failed:
            if case .failed(let message) = suggestion.state {
                Text(message).font(.caption).foregroundStyle(.red).accessibilityIdentifier("aiChat.suggest.error")
            }
            HStack(spacing: RenderTheme.Spacing.sm) {
                Button(action: onAdd) {
                    Label(L("追加"), systemImage: "plus").frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .primary))
                .disabled(suggestion.state == .saving)
                .accessibilityIdentifier("aiChat.suggest.add")
                Button(action: onFix) {
                    Label(L("直す"), systemImage: "pencil").frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .secondary))
                .disabled(suggestion.state == .saving)
                .accessibilityIdentifier("aiChat.suggest.fix")
            }
        }
    }
}
#endif
