package android.content

/** デスクトップでは使わない型の置き換え(共有する `SharedPreferencesKeyValueStore.of` の引数)。呼ぶと止まる。 */
abstract class Context {
    open val applicationContext: Context get() = error("android.content.Context is not available on desktop")

    open fun getSharedPreferences(name: String, mode: Int): SharedPreferences =
        error("android.content.Context is not available on desktop")

    companion object {
        const val MODE_PRIVATE = 0
    }
}
