# デスクトップ(Windows x64・arm64 / Linux x64 / 手元の macOS)の whisper.cpp(MIT)と JNI の口を 1 つのライブラリにする(#1751)。
# 版とハッシュは android/whisper/src/main/cpp/CMakeLists.txt と同じ物に固定する(上げる時は両方を変える)。
# 作り方は build.sh / build.ps1。出来たライブラリは out/<os>-<arch>/ に置き、git には入れない。
cmake_minimum_required(VERSION 3.22.1)
project(metamon_whisper LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

set(WHISPER_CPP_VERSION 1.9.5)
set(WHISPER_CPP_SHA256 ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720)

include(FetchContent)
FetchContent_Declare(
    whisper_cpp
    URL https://github.com/ggml-org/whisper.cpp/archive/refs/tags/v${WHISPER_CPP_VERSION}.tar.gz
    URL_HASH SHA256=${WHISPER_CPP_SHA256}
)

# 静的に 1 つのライブラリへまとめる。CPU だけ(GPU・OpenMP・例・テストは入れない)。
# GGML_NATIVE OFF: 作った機械の CPU に合わせない(配る物なので)。x64 は AVX2・FMA・F16C を前提にする(2013 年以降の CPU)。
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(WHISPER_BUILD_TESTS OFF CACHE BOOL "" FORCE)
set(WHISPER_BUILD_EXAMPLES OFF CACHE BOOL "" FORCE)
set(WHISPER_BUILD_SERVER OFF CACHE BOOL "" FORCE)
set(WHISPER_ALL_WARNINGS OFF CACHE BOOL "" FORCE)
set(GGML_NATIVE OFF CACHE BOOL "" FORCE)
set(GGML_OPENMP OFF CACHE BOOL "" FORCE)
set(GGML_METAL OFF CACHE BOOL "" FORCE)
set(GGML_BLAS OFF CACHE BOOL "" FORCE)
set(GGML_BUILD_TESTS OFF CACHE BOOL "" FORCE)
set(GGML_BUILD_EXAMPLES OFF CACHE BOOL "" FORCE)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

FetchContent_MakeAvailable(whisper_cpp)

# JDK の jni.h(JAVA_HOME の JDK を使う)
find_package(JNI REQUIRED COMPONENTS JVM)

add_library(metamon_whisper SHARED whisper_jni.cpp)
target_include_directories(metamon_whisper PRIVATE ${JNI_INCLUDE_DIRS})
target_link_libraries(metamon_whisper PRIVATE whisper)

if (MSVC)
    # VC++ の実行時ライブラリを中に入れる(配る PC に再頒布パッケージが無くても読める)
    set_property(TARGET metamon_whisper PROPERTY MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
elseif (APPLE)
    target_compile_options(metamon_whisper PRIVATE -fvisibility=hidden)
    target_link_options(metamon_whisper PRIVATE -Wl,-dead_strip)
else ()
    target_compile_options(metamon_whisper PRIVATE -fvisibility=hidden -ffunction-sections -fdata-sections)
    # libstdc++ を中に入れる(配る Linux の版の違いで読めなくならないように)
    target_link_options(metamon_whisper PRIVATE -Wl,--gc-sections -Wl,--exclude-libs,ALL -static-libstdc++ -static-libgcc)
endif ()
