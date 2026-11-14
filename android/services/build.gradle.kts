// :services — I/O(認証・API・音声など)。iOS の MetamonServices に当たる。Compose は使わない。
plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "jp.kz2601.metamon.services"
    compileSdk = 36

    defaultConfig {
        minSdk = 31
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    api(project(":dsl"))
    api(libs.kotlinx.coroutines.core)
    api(libs.okhttp)
    implementation(libs.kotlinx.coroutines.android)
    implementation(platform(libs.firebase.bom))
    implementation(libs.firebase.auth)
    implementation(libs.firebase.appcheck)
    implementation(libs.firebase.crashlytics)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}

// エラーコードの一覧が backend(backend/internal/httpx/codes.go)と揃っているかをテストで確かめる。
val backendCodesFile = rootDir.resolve("../backend/internal/httpx/codes.go")
val dslExamplesDir = rootDir.resolve("../docs/dsl/examples")

tasks.withType<Test>().configureEach {
    inputs.file(backendCodesFile).withPropertyName("backendCodes").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("metamon.backendCodesFile", backendCodesFile.absolutePath)
    // モック(#844)のテストはサンプル DSL を docs/dsl/examples から読む
    inputs.dir(dslExamplesDir).withPropertyName("dslExamples").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("metamon.dslExamplesDir", dslExamplesDir.absolutePath)
}
