import org.jetbrains.compose.desktop.application.dsl.TargetFormat
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

// :desktop — Windows / Linux / macOS のアプリ本体。DSL の読み込み・式・レコードの規則は Android と同じ :dsl を使う。
plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.compose.multiplatform)
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

dependencies {
    implementation(project(":dsl"))
    // API クライアント・記録の保存・送信待ち(android/services と同じソース・#1748 #1754)
    implementation(project(":services"))
    // 音声入力(マイク + whisper.cpp を JNI で・#1751)。既定は使わない(TAILORIT_VOICE=1 で使う)
    implementation(project(":whisper"))
    implementation(compose.desktop.currentOs)
    implementation(compose.material3)
    // icon の絵(#1742)。core だけ(extended は足さず、表で使う絵は Android の生成物を取り込む)。
    // androidx の版は Compose の ui を古い版で混ぜるので、Compose Multiplatform が出している同じ物を使う
    implementation("org.jetbrains.compose.material:material-icons-core:1.7.3")
    implementation(libs.zxing.core)
    // PDF の本文を読む・表を PDF に書く(#1753)。pdfbox + fontbox + pdfbox-io + commons-logging で約 3.8 MB
    implementation("org.apache.pdfbox:pdfbox:3.0.3")
    // Windows の電池・充電・省電力を kernel32 の GetSystemPowerStatus で読む(WindowsPowerReader)。jna だけで約 1.9 MB(jna-platform は足さない)。
    // LGPL-2.1-or-later と Apache-2.0 から選べるので Apache-2.0 を選ぶ(third-party-licenses.gradle.kts の electedLicenses)
    implementation("net.java.dev.jna:jna:5.19.1")

    // 画面のテスト(runComposeUiTest・#2415)。版は Compose Multiplatform plugin に揃う
    @OptIn(org.jetbrains.compose.ExperimentalComposeLibrary::class)
    testImplementation(compose.uiTest)
    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}

val dslDocsDir = rootDir.resolve("../docs/dsl")

// 初期の DSL は docs/dsl/examples をそのまま同梱する(iOS・Android と同じ)。jar の中では examples/<名前>.json。
tasks.processResources {
    from(dslDocsDir.resolve("examples")) {
        include("*.json")
        into("examples")
    }
}

// 適合テストは docs/dsl/conformance/cases(全 OS 共通の期待値)を読む。
tasks.test {
    inputs.dir(dslDocsDir.resolve("conformance")).withPropertyName("conformanceCases").withPathSensitivity(PathSensitivity.RELATIVE)
    inputs.dir(dslDocsDir.resolve("examples")).withPropertyName("dslExamples").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("metamon.dslDocsDir", dslDocsDir.absolutePath)
    // アプリ自身のライセンスの欄をリポジトリの LICENSE と突き合わせる(LicensesTest)
    val repoLicense = rootDir.resolve("../LICENSE")
    inputs.file(repoLicense).withPropertyName("repoLicense").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("metamon.repoLicense", repoLicense.absolutePath)
    // 文言・テーマはテストを動かす機械の言語・OS の設定に左右されないよう固定する(テストの中で切り替える)
    systemProperty("tailorit.lang", "ja")
    systemProperty("tailorit.theme", "light")
}

// Android の :renderer のうち Compose の状態しか使わない純 Kotlin(フォームの下書き・遷移の規則)は、android/ を変えずにそのまま取り込む。
val sharedRendererDir = rootDir.resolve("../android/renderer/src/main/kotlin/jp/kz2601/metamon/renderer")
val syncSharedRenderer by tasks.registering(Sync::class) {
    from(sharedRendererDir) {
        include("form/FormDraft.kt", "navigation/NavigationResolver.kt")
        // openUrlButton の to の {{fieldId}} を埋める(Android と同じ規則)
        include("components/actions/FormTemplate.kt")
        // icon の絵(android/scripts/gen-symbol-icons.py が icon-map.json から作った表・#1742)
        include("theme/icons/SymbolIconTable.kt", "theme/icons/VendoredIcons.kt")
    }
    // ライセンスの一覧の読み方・段落の分け方(Android の設定の「ライセンス」と同じ)
    from(rootDir.resolve("../android/app/src/main/kotlin/jp/kz2601/metamon/settings")) {
        include("ThirdPartyLicenses.kt")
        into("settings")
    }
    into(layout.buildDirectory.dir("generated/sharedRenderer"))
}
kotlin.sourceSets.named("main") { kotlin.srcDir(syncSharedRenderer) }

// 文言は Android の strings*.xml(values = 日本語・values-en = 英語)をビルド時にそのまま同梱し、同じキーで引く(#1759)。
// renderer と app で同じファイル名があるので置き場を分け、jar の中では一覧を引けないので index.txt を書く。
// デスクトップにしか無い文言は src/main/resources/i18n/desktop の同じ形の XML に置く(キーは desktop_ で始める)。
val androidResDir = rootDir.resolve("../android")
val syncAndroidStrings by tasks.registering(Sync::class) {
    listOf("renderer", "app").forEach { module ->
        from(androidResDir.resolve("$module/src/main/res")) {
            include("values/strings*.xml", "values-en/strings*.xml")
            into("i18n/$module")
        }
    }
    into(layout.buildDirectory.dir("generated/androidStrings"))
    doLast {
        val root = destinationDir.resolve("i18n")
        val files = root.walkTopDown().filter { it.isFile && it.extension == "xml" }.map { it.relativeTo(root).invariantSeparatorsPath }.sorted()
        root.resolve("index.txt").writeText(files.joinToString("\n", postfix = "\n"))
    }
}
sourceSets.named("main") { resources.srcDir(syncAndroidStrings) }

// 同梱するライセンスの一覧(licenses/third_party_licenses.json)。作り方は third-party-licenses.gradle.kts
apply(from = "third-party-licenses.gradle.kts")
sourceSets.named("main") { resources.srcDir(tasks.named("thirdPartyLicenses")) }

// icon の名前の表(SF Symbols → Material)は docs/dsl/icon-map.json をそのまま同梱して読む(#1742)。
val syncIconMap by tasks.registering(Sync::class) {
    from(dslDocsDir) {
        include("icon-map.json")
        into("icons")
    }
    into(layout.buildDirectory.dir("generated/iconMap"))
}
sourceSets.named("main") { resources.srcDir(syncIconMap) }

// 手元の確かめ: TAILORIT_API=stg TAILORIT_DEV_TOKEN=<ID トークン> ./gradlew :desktop:listApps(stg のアプリの一覧を出す)
tasks.register<JavaExec>("listApps") {
    group = "verification"
    description = "stg の appconfig からアプリの一覧を出す(手元の確かめ用・#1748)"
    classpath = sourceSets["main"].runtimeClasspath
    mainClass.set("jp.kz2601.metamon.desktop.net.ListAppsKt")
}

compose.desktop {
    application {
        mainClass = "jp.kz2601.metamon.desktop.MainKt"
        nativeDistributions {
            // 配布の形(MSIX・AppImage / Flatpak)と署名は後の Issue で決める。今は jpackage の既定の形だけ。
            targetFormats(TargetFormat.Msi, TargetFormat.Deb, TargetFormat.Rpm, TargetFormat.Dmg)
            packageName = "Tailor It"
            packageVersion = "0.1.0"
            // 読み上げ(Windows の Java Access Bridge)を同梱の実行環境に入れる(#1759)
            modules("jdk.accessibility")
            // whisper.cpp の JNI のライブラリ(whisper/native/build.sh・build.ps1 で out/<os>-<arch>/ に作る・#1751)を配る形に入れる。
            // 作っていなければ入らず、音声入力は「この PC では用意できていません」になる。モデルは入れない(初回に落とす)
            appResourcesRootDir.set(project(":whisper").layout.projectDirectory.dir("native/out"))
        }
    }
}
