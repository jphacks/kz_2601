import Foundation

/// 設定の「詳細(サポート用)」に出す値(#279)。uid・モデル名・接続先・ビルド番号は 1 画面目に出さず、ここにまとめる。
struct SupportInfo: Equatable {
    struct Row: Equatable {
        let label: String
        let value: String
    }

    var uid: String?
    var voiceModel: String
    var aiEndpoint: String
    var appConfigEndpoint: String
    var appVersion: String
    var build: String
    var appRevision: Int

    var rows: [Row] {
        var out: [Row] = []
        if let uid { out.append(Row(label: L10n.tr("ユーザー ID"), value: uid)) }
        out += [
            Row(label: L10n.tr("音声モデル"), value: voiceModel),
            Row(label: "ai", value: aiEndpoint),
            Row(label: "appconfig", value: appConfigEndpoint),
            Row(label: L10n.tr("バージョン"), value: "\(appVersion) (\(build))"),
            Row(label: L10n.pick(L10n.tr("今のアプリの版"), review: L10n.tr("今の記録帳の版")), value: "v\(appRevision)"),
        ]
        return out
    }

    /// 問い合わせに貼る用。1 行 1 項目。
    var clipboardText: String { rows.map { "\($0.label): \($0.value)" }.joined(separator: "\n") }

    /// アカウント欄の 1 行(識別子は出さない)
    static func accountStatus(isAnonymous: Bool) -> String {
        isAnonymous ? L10n.tr("匿名で利用中") : L10n.tr("Apple でサインイン中")
    }
}
