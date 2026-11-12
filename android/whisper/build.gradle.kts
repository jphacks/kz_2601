// :whisper — 端末内の文字起こし(whisper.cpp を NDK でビルドして JNI で呼ぶ・#1180)。iOS の WhisperKit に当たる。
// whisper.cpp の版と取得元は src/main/cpp/CMakeLists.txt(版とハッシュで固定)。モデルは APK に入れず、初回に落とす(WhisperModels)。
plugins {
    alias(libs.plugins.android.library)
}

android {
    namespace = "jp.kz2601.metamon.whisper"
    compileSdk = 36
    // 手元・CI で同じ NDK を使う(無ければ AGP が取ってくる)
    ndkVersion = "27.0.12077973"

    defaultConfig {
        minSdk = 31
        // 実機(arm64)とエミュレータ(x86_64)だけ。32bit は入れない(APK を増やさない・速さが足りない)
        ndk {
            abiFilters += listOf("arm64-v8a", "x86_64")
        }
        externalNativeBuild {
            cmake {
                arguments += listOf("-DANDROID_STL=c++_static")
            }
        }
    }

    externalNativeBuild {
        cmake {
            path = file("src/main/cpp/CMakeLists.txt")
            version = "3.22.1"
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    api(libs.kotlinx.coroutines.core)
    implementation(libs.okhttp)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}
