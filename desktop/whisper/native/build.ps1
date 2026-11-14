# whisper.cpp + JNI の口を Windows(x64・arm64)向けに作る(#1751)。
# 出来た物は out\windows-<arch>\metamon_whisper.dll(Compose の appResourcesRootDir の形)。git には入れない。
# 要る物: Visual Studio 2022 の「C++ によるデスクトップ開発」(MSVC・CMake)・JDK(JAVA_HOME)・取得のための通信(初回だけ)。
# 使い方(PowerShell で): desktop\whisper\native\build.ps1 [-Arch x64|arm64]
param([ValidateSet("x64", "arm64")][string]$Arch = $(if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "x64" }))
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$target = "windows-$Arch"
$platform = if ($Arch -eq "arm64") { "ARM64" } else { "x64" }
cmake -S . -B "build\$target" -A $platform
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
cmake --build "build\$target" --config Release --target metamon_whisper
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

New-Item -ItemType Directory -Force -Path "out\$target" | Out-Null
Copy-Item "build\$target\Release\metamon_whisper.dll" "out\$target\metamon_whisper.dll" -Force
Write-Output "out\$target\metamon_whisper.dll"
