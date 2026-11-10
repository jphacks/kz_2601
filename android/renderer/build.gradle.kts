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
    implementation(platform(libs.compose.bom))
    api(libs.compose.ui)
    api(libs.compose.material3)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.browser)
    implementation(libs.compose.material.icons.core)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)

    testImplementation(libs.junit)
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
