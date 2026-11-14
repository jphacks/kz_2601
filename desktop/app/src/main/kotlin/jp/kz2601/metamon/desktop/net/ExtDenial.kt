package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ErrorCodes
import jp.kz2601.metamon.services.http.PaidFeature
import jp.kz2601.metamon.services.http.ProRequired

/**
 * 外の API(#1493)の op が断った理由(#2001・iOS の `ExtDenial`)。読み方は android/services の [ErrorCodes]・[ProRequired] に合わせる。
 * 外の API の断りでなければ [from] は null(ふつうのエラーの扱いに任せる)。
 */
sealed interface ExtDenial {
    /** 409 `ext_not_connected`。`details.provider` が無い古いサーバーは null */
    data class NotConnected(val provider: String?) : ExtDenial

    /** 402 `pro_required`(外の API は Pro だけ) */
    data class NeedsPro(val pro: ProRequired) : ExtDenial

    /** 402 `coins_insufficient` / `allowance_exhausted` / `overflow_cap_reached` */
    data class Coins(val kind: CoinKind, val required: Int? = null, val balance: Int? = null, val payer: String? = null) : ExtDenial {
        /** 自分のコインで直せるか(共有アプリの作成者が払う時は false) */
        val payerIsSelf: Boolean get() = payer == null || payer == "self"
    }

    /** 503 `ext_unavailable` */
    data object Unavailable : ExtDenial

    /** 503 `ext_busy` */
    data object Busy : ExtDenial

    enum class CoinKind(val wire: String) {
        COINS_INSUFFICIENT(ErrorCodes.COINS_INSUFFICIENT),
        ALLOWANCE_EXHAUSTED(ErrorCodes.ALLOWANCE_EXHAUSTED),
        OVERFLOW_CAP_REACHED(ErrorCodes.OVERFLOW_CAP_REACHED),
    }

    companion object {
        /** ext の code は code だけで見る(iOS と同じ)。402 は status も見る。 */
        fun from(error: Throwable): ExtDenial? {
            val api = error as? ApiError ?: return null
            if (api.code == ErrorCodes.EXT_NOT_CONNECTED) return NotConnected(api.detailString("provider")?.takeIf { it.isNotEmpty() })
            ProRequired.from(api)?.let { return NeedsPro(it) }
            if (api.status == 402) {
                CoinKind.entries.firstOrNull { it.wire == api.code }?.let { kind ->
                    return Coins(kind, api.detailNumber("required")?.toInt(), api.detailNumber("balance")?.toInt(), api.detailString("payer"))
                }
            }
            return when (api.code) {
                ErrorCodes.EXT_UNAVAILABLE -> Unavailable
                ErrorCodes.EXT_BUSY -> Busy
                else -> null
            }
        }
    }
}

/** 外の API の断りの「次にすること」(iOS の `ExtDenialRoute`)。画面はまだ無い。 */
sealed interface ExtDenialRoute {
    /** つなぐ案内 */
    data class Connect(val provider: String) : ExtDenialRoute

    /** Pro の案内。デスクトップでは買えないので、スマホのアプリで Pro にする旨だけ出す */
    data class ProInfo(val features: List<PaidFeature>) : ExtDenialRoute

    /** コインの残高の画面 */
    data class CoinBalance(val denial: ExtDenial.Coins) : ExtDenialRoute

    /** 案内の画面を出さず 1 文だけ */
    data class Message(val reason: Reason) : ExtDenialRoute

    enum class Reason {
        /** つなぐ先が分からない(設定の「つないだサービス」へ) */
        NOT_CONNECTED_UNKNOWN,

        /** 共有アプリの作成者の Pro が要る(参加者は払っても直らない) */
        CREATOR_NEEDS_PRO,

        /** 端末は Pro なのにサーバーが知らない(Pro の案内を繰り返さない) */
        PRO_NOT_SYNCED,
        UNAVAILABLE,
        BUSY,
    }

    companion object {
        /** 外の API を使う有料機能(features が空の時に出す・iOS の `ConnectedServicesSection.paidFeature`) */
        const val EXT_FEATURE_ID = "externalServices"

        fun of(denial: ExtDenial, isPro: Boolean = false): ExtDenialRoute = when (denial) {
            is ExtDenial.NotConnected -> denial.provider?.let(::Connect) ?: Message(Reason.NOT_CONNECTED_UNKNOWN)
            is ExtDenial.NeedsPro -> when {
                denial.pro.creatorNeedsPro -> Message(Reason.CREATOR_NEEDS_PRO)
                isPro -> Message(Reason.PRO_NOT_SYNCED)
                else -> ProInfo(denial.pro.features.ifEmpty { listOf(PaidFeature(EXT_FEATURE_ID, EXT_FEATURE_ID)) })
            }
            is ExtDenial.Coins -> CoinBalance(denial)
            ExtDenial.Unavailable -> Message(Reason.UNAVAILABLE)
            ExtDenial.Busy -> Message(Reason.BUSY)
        }

        /** エラーから行き先。外の API の断りでなければ null。 */
        fun of(error: Throwable, isPro: Boolean = false): ExtDenialRoute? = ExtDenial.from(error)?.let { of(it, isPro) }
    }
}
