import com.google.gms.googleservices.GoogleServicesPlugin.GoogleServicesPluginConfig
import com.google.gms.googleservices.GoogleServicesPlugin.MissingGoogleServicesStrategy

// :app — 起動・組み立て(DI)・flavor(stg / prod)。ロジックは下のモジュールに置く。
plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

// google-services.json はコミットしない(#356・#840)。手元にある時だけ google-services と Crashlytics のプラグインを当てる
// (iOS の select-firebase-plist.sh の「あれば入れる」と同じ)。無ければ Firebase を初期化せず、認証はモックで動く。
val hasGoogleServicesJson = fileTree("src") { include("*/google-services.json") }.files.isNotEmpty() ||
    file("google-services.json").exists()
if (hasGoogleServicesJson) {
    apply(plugin = libs.plugins.google.services.get().pluginId)
    apply(plugin = libs.plugins.firebase.crashlytics.get().pluginId)
    // prod だけある時など、json の無い flavor は警告だけにする
    extensions.configure<GoogleServicesPluginConfig> {
        missingGoogleServicesStrategy = MissingGoogleServicesStrategy.WARN
    }
}

/** gradle.properties / ~/.gradle/gradle.properties / -P の値(無ければ空)。秘密ではないが repo には書かない。 */
fun stringProperty(name: String): String = (findProperty(name) as String?).orEmpty()

android {
    namespace = "jp.kz2601.metamon"
    compileSdk = 36

    defaultConfig {
        applicationId = "jp.kz2601.metamon"
        minSdk = 31
        targetSdk = 36
        versionCode = 1
        versionName = "0.1.0"
    }

    // 接続先は iOS の ios/project.yml(METAMON_AI_BASE_URL / METAMON_APPCONFIG_BASE_URL)と同じ。
    flavorDimensions += "env"
    productFlavors {
        create("stg") {
            dimension = "env"
            applicationIdSuffix = ".stg"
            versionNameSuffix = "-stg"
            buildConfigField("String", "METAMON_ENV", "\"stg\"")
            buildConfigField("String", "AI_BASE_URL", "\"https://ai-stg-378953239426.asia-northeast1.run.app\"")
            buildConfigField("String", "APPCONFIG_BASE_URL", "\"https://appconfig-stg-378953239426.asia-northeast1.run.app\"")
            resValue("string", "app_name", "Tailor It β")
            // Google サインインの Web クライアント ID。空なら google-services.json の default_web_client_id を使う
            buildConfigField("String", "GOOGLE_WEB_CLIENT_ID", "\"${stringProperty("metamon.stg.googleWebClientId")}\"")
        }
        create("prod") {
            dimension = "env"
            buildConfigField("String", "METAMON_ENV", "\"prod\"")
            buildConfigField("String", "AI_BASE_URL", "\"https://ai-prod-378953239426.asia-northeast1.run.app\"")
            buildConfigField("String", "APPCONFIG_BASE_URL", "\"https://appconfig-prod-378953239426.asia-northeast1.run.app\"")
            resValue("string", "app_name", "Tailor It")
            buildConfigField("String", "GOOGLE_WEB_CLIENT_ID", "\"${stringProperty("metamon.prod.googleWebClientId")}\"")
        }
    }

    // 初期の DSL は docs/dsl/examples をそのまま同梱する(iOS と同じ)。
    sourceSets {
        getByName("main") {
            assets.directories.add(rootDir.resolve("../docs/dsl/examples").path)
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
        buildConfig = true
        resValues = true
    }

    // アプリ内で言語を切り替える(#843)。Play の言語ごとの分割で切り替え先の文言が端末に無い、を防ぐ
    bundle {
        language {
            enableSplit = false
        }
    }
}

dependencies {
    implementation(project(":dsl"))
    implementation(project(":renderer"))
    implementation(project(":services"))

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.process)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.core)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)
    implementation(platform(libs.firebase.bom))
    implementation(libs.firebase.auth)
    implementation(libs.firebase.appcheck)
    // App Check: 配布版は Play Integrity、debug ビルドだけ debug provider(src/debug・src/release で出し分け)
    releaseImplementation(libs.firebase.appcheck.playintegrity)
    debugImplementation(libs.firebase.appcheck.debug)
    implementation(libs.androidx.credentials)
    implementation(libs.androidx.credentials.play.services.auth)
    implementation(libs.googleid)
    implementation(libs.firebase.messaging)
    implementation(libs.play.app.update.ktx)
    implementation(libs.play.services.nearby)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}

// ランタイムのテストは docs/dsl/examples(全 OS 共通の例)をそのまま読む。
val dslExamplesDir = rootDir.resolve("../docs/dsl/examples")

tasks.withType<Test>().configureEach {
    inputs.dir(dslExamplesDir).withPropertyName("dslExamples").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("metamon.examplesDir", dslExamplesDir.absolutePath)
}
