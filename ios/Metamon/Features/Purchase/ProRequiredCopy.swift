import MetamonServices

/// サーバーの 402 `pro_required`(#316)の文言。審査用の版でも判定は残すので、このファイルは `APP_STORE_REVIEW` で外さない
/// (その版は有料の機能を作れず、共有にも参加しないので、出るのは古いアカウントの共有アプリに書いた時くらい)。
extension ProRequired {
    /// 生成・直しが保存されなかった時(課金画面を閉じた後・課金の無い版)
    var notSavedMessage: String {
        let names = features.map(\.title).joined(separator: "・")
        if names.isEmpty { return L10n.tr("Tailor It Pro が必要な機能が入るため、保存しませんでした") }
        return L10n.tr("「\(names)」は Tailor It Pro が必要なため、保存しませんでした")
    }

    /// 共有の記録に書けなかった時。参加者は作成者の Pro が要る(払っても直らない)
    var writeMessage: String {
        creatorNeedsPro
            ? L10n.tr("このアプリを作った人が Tailor It Pro ではないため、共有のデータに書き込めません(見ることはできます)")
            : L10n.tr("共有のデータに書き込むには Tailor It Pro が必要です(見ることはできます)")
    }
}
