package jp.kz2601.metamon.desktop.auth

/** つながっているサインインの方法(iOS の `SignInProvider`)。password・phone などは扱わない。並びは iOS と同じ(Apple → Google)。 */
enum class SignInProvider(val providerId: String) {
    APPLE("apple.com"),
    GOOGLE("google.com"),
}

/**
 * 設定の「サインインの方法」に並べる物(`accounts:lookup` の `providerUserInfo` から・読むだけ。足す・外すはしない)。
 * 匿名の人には出さない([rows] が空)。
 */
data class LinkedProviders(
    val providers: List<SignInProvider>,
    /** `providerUserInfo` がひとつも無い(匿名)。password のアカウントは含まない */
    val isAnonymous: Boolean,
) {
    /** 画面に並べる行。つながっている方法だけ(つながっていない Google などは出さない) */
    val rows: List<SignInProvider> get() = if (isAnonymous) emptyList() else providers

    companion object {
        /** Firebase の providerId の並びから、知っている物だけを iOS と同じ順で取り出す。 */
        fun from(providerIds: List<String>): LinkedProviders = LinkedProviders(
            providers = SignInProvider.entries.filter { it.providerId in providerIds },
            isAnonymous = providerIds.isEmpty(),
        )
    }
}
