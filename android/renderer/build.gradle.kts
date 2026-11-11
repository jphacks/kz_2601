// :renderer — DSL を Compose で描く。iOS の MetamonRenderer に当たる。:services を知らない(能力は DSLRuntime 経由)。
plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.compose)
}

android {
    namespace = "jp.kz2601.metamon.renderer"
    compileSdk = 36

    defaultConfig {
        minSdk = 31
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
    }
}

dependencies {
    api(project(":dsl"))
    api(libs.kotlinx.coroutines.core)
    implementation(platform(libs.compose.bom))
    api(libs.compose.ui)
    api(libs.compose.material3)
    implementation(libs.androidx.activity.compose)
    // photoField のカメラの保存先(FileProvider)
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.browser)
    // https の画像。通信は :services と同じ OkHttp(版は libs の okhttp に揃える)
    implementation(libs.coil.compose)
    implementation(libs.coil.network.okhttp)
    implementation(libs.okhttp)
    // videoPlayer の再生
    implementation(libs.media3.exoplayer)
    implementation(libs.media3.ui)
    // dashcam のループ録画(#872)
    implementation(libs.camerax.camera2)
    implementation(libs.camerax.lifecycle)
    implementation(libs.camerax.video)
    implementation(libs.camerax.view)
    // mapView / locationPicker の地図(#867)。タイルの配信元は components/map/MapStyle.kt の 1 か所
    implementation(libs.maplibre.android.sdk.opengl)
    implementation(libs.compose.material.icons.core)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.play.services.code.scanner)
    implementation(libs.zxing.core)
    // fileImport の PDF(#1181)。暗号化した PDF は読まない(iOS の isLocked と同じく断る)ので BouncyCastle は入れない
    implementation(libs.pdfbox.android) { exclude(group = "org.bouncycastle") }
    implementation(libs.play.services.mlkit.text.recognition.japanese)
    debugImplementation(libs.compose.ui.tooling)

    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
}

// 適合テストは docs/dsl/conformance/cases(全 OS 共通の期待値)と、case が参照する docs/dsl/examples をそのまま読む。
val dslDocsDir = rootDir.resolve("../docs/dsl")
val backendDir = rootDir.resolve("../backend")

tasks.withType<Test>().configureEach {
    inputs.dir(dslDocsDir.resolve("conformance")).withPropertyName("conformanceCases").withPathSensitivity(PathSensitivity.RELATIVE)
    inputs.dir(dslDocsDir.resolve("examples")).withPropertyName("dslExamples").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("metamon.dslDocsDir", dslDocsDir.absolutePath)
    // アイコンの対応表の漏れ検査(SymbolIconTableTest): 表・backend の AI のカタログ・部品のソースが書く名前を読む。
    inputs.file(dslDocsDir.resolve("icon-map.json")).withPropertyName("iconMap").withPathSensitivity(PathSensitivity.RELATIVE)
    inputs.file(dslDocsDir.resolve("dsl.schema.json")).withPropertyName("dslSchema").withPathSensitivity(PathSensitivity.RELATIVE)
    inputs.files(fileTree(backendDir) { include("**/*.go") }).withPropertyName("backendGo").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("metamon.backendDir", backendDir.absolutePath)
    systemProperty("metamon.rendererSrcDir", projectDir.resolve("src/main/kotlin").absolutePath)
}
