# 最低の版・最新の版・端末の強制更新(#302・#333・#906・#981)

> [バックエンド設計](../architecture.md) から分けた機能の文書。環境変数の一覧と変え方は [runtime-config.md の「運用で変える値」](../../infra/runtime-config.md#運用で変える値)、App Check の対象外であることは [app-check.md](../../infra/app-check.md)。API は [openapi-appconfig.yaml](../openapi-appconfig.yaml) の `/client-config`。コードは `backend/internal/appconfig/clientconfig.go`・`notice.go`。

## 目次
- [1. 何をするか](#1-何をするか)
- [2. 返す項目](#2-返す項目)
- [3. 環境変数](#3-環境変数)
- [4. 端末の振る舞い](#4-端末の振る舞い)
- [5. 出す時の順番](#5-出す時の順番)
- [6. 決めること](#6-決めること)

## 1. 何をするか

- `GET /v1/client-config` は、OS ごとに「これより古い版は先へ進めない(最低の版)」「これより古い版には更新を案内する(最新の版)」と、アプリ内のお知らせ(#333)を返す
- 認証も App Check も要らない(サインインの前にも読む・古い版に「アップデートしてください」を出すため)。利用者のデータは持たない
- 値は appconfig の起動時に環境変数から 1 回読む。変えたら新しいリビジョンが要る(`--update-env-vars` で入る)
- `Cache-Control: public, max-age=300`(端末・途中のキャッシュで最大 5 分遅れる)

## 2. 返す項目

値が無い項目は省く(= その OS は強制しない)。

| 項目 | 中身 |
|---|---|
| `androidStoreUrl` | Android の更新先(`https` / `market` だけ) |
| `iosAppStoreUrl` | iOS の更新先(`https` / `itms-apps` だけ) |
| `latestAndroidBuild` | Android の最新の版(versionCode) |
| `latestIOSBuild` | iOS の最新の版(CFBundleVersion) |
| `latestMacOSBuild` | Mac の最新の版(CFBundleVersion) |
| `macAppStoreUrl` | Mac の更新先(`https` / `macappstore` だけ) |
| `minAndroidBuild` | Android の最低の版(versionCode) |
| `minIOSBuild` | iOS の最低の版(CFBundleVersion) |
| `minMacOSBuild` | Mac の最低の版(CFBundleVersion) |
| `notice` | アプリ内のお知らせ `{id, severity: info\|warning, message: {ja, en?}, linkUrl?, minIOSBuild?, maxIOSBuild?}`(#333) |

- 版は正の整数だけ。0・負・数字でない値は警告ログ(`appconfig: ignoring invalid build number`)を出して無いことにする
- 最新が最低より小さい時は最新を最低に合わせる
- 更新先の URL がスキームか host の点で違えば無いことにする(端末はアプリ側の既定の URL を使う)
- `NOTICE_JSON` が壊れていても `notice` が無いだけで、版の項目は返す

## 3. 環境変数

すべて appconfig。既定は無し(強制しない)。

| 変数 | 項目 |
|---|---|
| `ANDROID_STORE_URL` | `androidStoreUrl` |
| `IOS_APP_STORE_URL` | `iosAppStoreUrl` |
| `LATEST_ANDROID_BUILD` | `latestAndroidBuild` |
| `LATEST_IOS_BUILD` | `latestIOSBuild` |
| `LATEST_MACOS_BUILD` | `latestMacOSBuild` |
| `MAC_APP_STORE_URL` | `macAppStoreUrl` |
| `MIN_ANDROID_BUILD` | `minAndroidBuild` |
| `MIN_IOS_BUILD` | `minIOSBuild` |
| `MIN_MACOS_BUILD` | `minMacOSBuild` |
| `NOTICE_JSON` | `notice`(カンマを含むので `^@^` で区切る・[変え方](../../infra/runtime-config.md#変え方)) |

## 4. 端末の振る舞い

どの端末も **fail open**: 読めない・項目が無い・自分の版が読めない時は何もしない(backend が落ちていてもアプリは使える)。版は `12` / `1.2.3` の数字の並びを左から比べる(足りない桁は 0)。

| 端末 | 読む時 | 自分の版と使う項目 | 最低の版より古い | 最新の版より古い |
|---|---|---|---|---|
| Android | 起動時・前面に戻った時(`UpdateGate.kt`) | versionCode・`*Android*` だけ(iOS の値は使わない・#906) | 閉じられない「アップデートしてください」画面 | 閉じられるお知らせ(同じ latest には 1 起動で 1 回) |
| iOS | 起動時・前面に戻った時(`UpdateGate.swift`) | CFBundleVersion・`*IOS*` | 閉じられない「アップデートしてください」画面 | 閉じられるお知らせ(同じ latest には 1 起動で 1 回) |
| Mac | 起動時に 1 回(サービスのお知らせと同じ・`MacUpdateGate`) | CFBundleVersion・`*MacOS*`(iOS の値とは別・#981) | 閉じられない「アップデートしてください」のシート | 閉じられるお知らせ(同じ latest には 1 起動で 1 回) |
| Windows・Linux(`desktop/`) | 読まない | - | - | - |

- 「新しい版があります」は、起動時の同意・チュートリアル・シェアリンクの順番待ちが終わってから出す(先に出すと同意のシートが捨てられるため)
- 更新先: サーバーの URL → アプリの Info.plist(`MetamonAppStoreURL`)→ ストアで「Tailor It」を検索、の順(iOS・Mac)
- お知らせ(`notice`)は閉じた `id` を端末に覚え、同じ `id` は二度と出さない。変えたお知らせを出し直す時は新しい `id` にする。閉じられない更新画面の間は出さない。`minIOSBuild` / `maxIOSBuild` で出す iOS の版を絞れる
- 見た目の確認: 環境変数 `METAMON_FORCE_UPDATE=required|recommended`・`METAMON_FORCE_NOTICE=info|warning`(iOS・Mac)
- モック・審査用の版: 版の確認はモックでは読まない。お知らせは審査用の版でも出す(運用の機能)

## 5. 出す時の順番

最低の版を上げると、それより古い端末は先へ進めなくなる。下は推奨の順番(コードが強制するものではない)。

1. 新しい版がストアで全員に配られた(段階的な公開が終わった)ことを確かめる([staged-rollout.md](../../infra/staged-rollout.md))
2. まず `LATEST_*_BUILD` を上げて、閉じられる案内だけを出す
3. 古い版を止める理由(API の互換が切れる・App Check を `enforce` にする等)がある時だけ `MIN_*_BUILD` を上げる。App Check の時の順番は [app-check.md](../../infra/app-check.md)

## 6. 決めること

- Windows・Linux の版(`MIN_WINDOWS_BUILD`・`MIN_LINUX_BUILD` など)は [#2131](https://github.com/jphacks/kz_2601/issues/2131) で足す。足したらこの文書の 2〜4 章に行を足す
- `notice` の版の絞り込みは iOS の版だけ(`minIOSBuild` / `maxIOSBuild`)。Android・Mac でも絞るか(推測: 今は全部の版に出せば足りている)
