package jp.kz2601.metamon.services

/** 接続先(stg / prod)。値は :app の product flavor の BuildConfig から入る。 */
data class AppEnvironment(
    /** "stg" | "prod" */
    val name: String,
    val aiBaseUrl: String,
    val appConfigBaseUrl: String,
) {
    val isProduction: Boolean get() = name == "prod"
}
