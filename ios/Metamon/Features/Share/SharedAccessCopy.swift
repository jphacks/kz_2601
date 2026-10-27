import MetamonServices

/// 参加者(#254)まわりの 403 の文言。#378 の「このデータは作った人だけが変更できます」とは別。
/// records の 403 は審査用の版でも起きうるので、このファイルは `APP_STORE_REVIEW` で外さない。
extension SharedAccessDenial {
    var message: String {
        switch self {
        case .notMember: L10n.tr("この共有に参加していません。招待リンクから参加してください")
        case .inviterLeft: L10n.tr("このリンクを送った人はもう参加していないため、参加できません。参加している人に新しいリンクをもらってください")
        case .membersFull: L10n.tr("参加している人が上限(100 人)に達しているため、参加できません")
        }
    }
}
