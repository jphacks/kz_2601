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
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)

    testImplementation(libs.junit)
}

// 適合テストは docs/dsl/conformance/cases(全 OS 共通の期待値)と、case が参照する docs/dsl/examples をそのまま読む。
val dslDocsDir = rootDir.resolve("../docs/dsl")

tasks.withType<Test>().configureEach {
    inputs.dir(dslDocsDir.resolve("conformance")).withPropertyName("conformanceCases").withPathSensitivity(PathSensitivity.RELATIVE)
    inputs.dir(dslDocsDir.resolve("examples")).withPropertyName("dslExamples").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("metamon.dslDocsDir", dslDocsDir.absolutePath)
}
