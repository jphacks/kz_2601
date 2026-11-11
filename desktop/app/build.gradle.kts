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
    implementation(compose.desktop.currentOs)
    implementation(compose.material3)
    implementation(libs.zxing.core)
    // PDF の本文を読む・表を PDF に書く(#1753)。pdfbox + fontbox + pdfbox-io + commons-logging で約 3.8 MB
    implementation("org.apache.pdfbox:pdfbox:3.0.3")

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
}

// Android の :renderer のうち Compose の状態しか使わない純 Kotlin(フォームの下書き・遷移の規則)は、android/ を変えずにそのまま取り込む。
val sharedRendererDir = rootDir.resolve("../android/renderer/src/main/kotlin/jp/kz2601/metamon/renderer")
val syncSharedRenderer by tasks.registering(Sync::class) {
    from(sharedRendererDir) {
        include("form/FormDraft.kt", "navigation/NavigationResolver.kt")
    }
    into(layout.buildDirectory.dir("generated/sharedRenderer"))
}
kotlin.sourceSets.named("main") { kotlin.srcDir(syncSharedRenderer) }

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
        }
    }
}
