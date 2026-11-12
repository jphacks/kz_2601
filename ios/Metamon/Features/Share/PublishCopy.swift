// 審査用ビルド(APP_STORE_REVIEW・#245)ではシェアごと外す。
// 「みんなに公開」の文言と入力の読み方。iOS の ShareAppSheet と Mac の MacPublishSection(#2070)で共有する。
#if !APP_STORE_REVIEW
import Foundation

/// 「みんなに公開」の前に出す注意(Android #1297 と同じ 4 つ)。
enum PublishConfirmCopy {
    struct Point: Equatable {
        var icon: String
        var text: String
    }

    static func title(appName: String) -> String { L10n.tr("「\(appName)」を公開しますか?") }

    static var points: [Point] {
        [
            Point(icon: "magnifyingglass", text: L10n.tr("「探す」から誰でも見つけて、自分のアプリとして入れられます")),
            Point(icon: "lock", text: L10n.tr("あなただけの記録は含まれません")),
            Point(icon: "person.crop.circle.badge.exclamationmark", text: L10n.tr("名前・説明・タグに、住所や電話番号などの個人の情報を書かないでください")),
            Point(icon: "globe.badge.chevron.backward", text: L10n.tr("このシェアの画面から、いつでも公開をやめられます")),
        ]
    }
}

/// 公開の入力の読み方と失敗の文言
enum PublishCopy {
    /// タグの欄を区切る(半角・全角スペース、「,」「、」)
    static func tags(from text: String) -> [String] {
        text.split(whereSeparator: { $0 == " " || $0 == "　" || $0 == "," || $0 == "、" }).map(String.init)
    }

    /// 表示できない言葉があった項目(#321)。どの欄かを名前で出す
    static func textNotAllowedMessage(field: String?) -> String {
        switch field {
        case "description": ErrorCopy.textNotAllowed(fieldName: L10n.tr("ひとこと説明"))
        case "tags": ErrorCopy.textNotAllowed(fieldName: L10n.tr("タグ"))
        case "name": ErrorCopy.textNotAllowed(fieldName: L10n.tr("アプリの名前"))
        case "authorName": ErrorCopy.textNotAllowed(fieldName: L10n.tr("表示名"))
        default: ErrorCopy.textNotAllowed
        }
    }
}
#endif
