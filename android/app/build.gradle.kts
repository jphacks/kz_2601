import com.google.gms.googleservices.GoogleServicesPlugin.MissingGoogleServicesStrategy

// :app — 起動・組み立て(DI)・flavor(stg / prod)。ロジックは下のモジュールに置く。
plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.google.services)
}

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
        }
        create("prod") {
            dimension = "env"
            buildConfigField("String", "METAMON_ENV", "\"prod\"")
            buildConfigField("String", "AI_BASE_URL", "\"https://ai-prod-378953239426.asia-northeast1.run.app\"")
            buildConfigField("String", "APPCONFIG_BASE_URL", "\"https://appconfig-prod-378953239426.asia-northeast1.run.app\"")
            resValue("string", "app_name", "Tailor It")
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
}

// google-services.json はコミットしない(#356)。無ければ警告だけ出してビルドし、実行時はモックの認証で動く。
googleServices {
    missingGoogleServicesStrategy = MissingGoogleServicesStrategy.WARN
}

dependencies {
    implementation(project(":dsl"))
    implementation(project(":renderer"))
    implementation(project(":services"))

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.material3)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)
    implementation(platform(libs.firebase.bom))
    implementation(libs.firebase.auth)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
}
