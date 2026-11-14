# デスクトップ版の端末の保存

デスクトップ版(`desktop/`)が端末のどこに何を置き、どう書き、いつ消すか。コードは `desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop/store/`(以下 `store/`)と、そこから使う `android/services` の `services/local/`(同じソースを取り込む。[architecture.md](architecture.md))。動かし方は [`desktop/README.md`](../../desktop/README.md)。

## 目次

- [置き場(AppDataDir)](#置き場appdatadir)
- [何をどのファイルに置くか](#何をどのファイルに置くか)
- [書き込み: 一時ファイルから入れ替える](#書き込み-一時ファイルから入れ替える)
- [壊れたファイル](#壊れたファイル)
- [送信待ち(SessionPersistence)](#送信待ちsessionpersistence)
- [消す時: 消える物と残る物](#消す時-消える物と残る物)
- [決めること](#決めること)

## 置き場(AppDataDir)

`AppDataDir.resolve()` が OS ごとに決める。

| OS | 置き場 |
|---|---|
| Windows | `%APPDATA%\TailorIt`(`APPDATA` が無ければ `~/AppData/Roaming/TailorIt`) |
| Linux | `$XDG_DATA_HOME/tailorit`(絶対パスの時だけ。無ければ `~/.local/share/tailorit`) |
| macOS | `~/Library/Application Support/TailorIt` |

- `-Dtailorit.dataDir=<フォルダ>`(`AppDataDir.OVERRIDE_PROPERTY`)で差し替える。テストや、別の人として同時に起動する時に使う
- 同じ置き場ではアプリを 1 つしか起動しない(`SingleInstance`。置き場の `.lock` をロックする)

## 何をどのファイルに置くか

`DesktopStore` が置き場の下を次のように使う。`DesktopStore(root = null)` はメモリだけ(テスト用)。

| パス | 中身 | 書くクラス |
|---|---|---|
| `settings.json` | 設定。今は最後に開いたアプリ(鍵 `tailorit.lastAppId`) | `JsonKeyValueStore`(`DesktopStore.settings`) |
| `consent.json` | AI への送信の同意・クラッシュ報告の設定。鍵は iOS・Android と同じ | `JsonKeyValueStore` を `AiConsentStore`・`CrashReportingPreference` が使う |
| `records/<appId>/<entity>.json` | 記録のキャッシュ(送っていない印 `pendingOp` つき) | `RecordStore` |
| `queue/queue.json` | 送信待ちの列と、仮 id → サーバー id の対応 | `SyncQueue`(`DesktopStore.queue()`) |
| `documents/<appId>.json` | 取得した DSL の封筒(オフラインで起動するため) | `DocumentCache` |
| `models/` | 音声入力のモデル(初回に落とす) | `VoiceModels.dir()`(`voice/Transcriber.kt`) |
| `.lock`・`.instance` | 2 つ目の起動を止める・前に出させるための小さなファイル | `SingleInstance` |

- `settings.json`・`consent.json` は 1 ファイルの JSON オブジェクト(値は文字列・数・真偽だけ)。鍵の名前順で書く
- `<appId>`・`<entity>` はファイル名に使えない文字を `_` にする(`RecordStore.safe`)
- ウィンドウの大きさ・位置は置き場ではなく `java.util.prefs`(Windows はレジストリ、Linux は `~/.java`)に置く(`WindowBounds`・`Main.kt`)
- 貼った写真・引き出す一時ファイル・カレンダーに渡すファイルは置き場の外の `UserDirs`(`io/UserDirs.kt`。Linux は `~/.cache`、他は一時フォルダの下)

## 書き込み: 一時ファイルから入れ替える

書く途中で落ちても前の中身が残るように、同じフォルダの一時ファイルに書いてから入れ替える。

| 書く物 | 関数 | 一時ファイル |
|---|---|---|
| `settings.json`・`consent.json` | `writeAtomically`(`store/JsonKeyValueStore.kt`) | `<名前>.<数字>.tmp`(毎回別の名前) |
| 記録・送信待ち | `AtomicFile.write`(`services/local/RecordStore.kt`) | `<名前>.tmp` |

`writeAtomically` は次の順で書く。

1. 一時ファイルに書き、`force` でディスクまで書き切る(電源が落ちても空のファイルに入れ替わらない)
2. `ATOMIC_MOVE` で入れ替える。Windows でウイルス対策や検索の索引が開いていて `FileSystemException` になった時は、少し待って `MOVE_RETRIES`(3)回まで試し直す
3. 入れ替えられなかった一時ファイルは消す

落ちて残った `*.tmp` は `sweepTempFiles` が消す。呼ぶのは開く時(`DesktopStore.open`)と全部消す時(`DesktopStore.wipe`)。置き場は 1 プロセスしか使わない(上の `.lock`)ので、開く時に消してよい。

## 壊れたファイル

| ファイル | 読めない時 |
|---|---|
| `settings.json`・`consent.json` | `<名前>.corrupt` に移して空から始める(上書きで消さず、後で調べられるようにする。前の `.corrupt` は置き換える) |
| `records/…` | 空として読む(サーバーから引き直せる) |
| `queue/queue.json` | 空として読む |

`.corrupt` は「この端末のデータを消す」で消す(下の `DeviceWipe`)。

## 送信待ち(SessionPersistence)

`SessionPersistence` が画面の記録(`MockStore`)と端末の保存をつなぐ。

- 画面の記録が変わるたびに前後の差(作成・更新・削除)を `RecordStore` に書く。書き込みは 1 本の列で順に流し、画面を止めない(`idle()` で書き終わりを待てる)
- 送り先があるのは `TAILORIT_API=stg` と `TAILORIT_DEV_TOKEN` が両方ある時だけ(`SessionPersistence.client`)。その時は同じ差を `SyncQueue` に積んで送る。無ければ端末に残すだけで通信しない
- 新しい記録は仮 id(`LocalRecord.temporaryId`)で作る。送れた後も画面には仮 id のまま出す(`screenId`)
- 送っていない作成を消した時は、送らずに端末から消す(`SyncQueue.EnqueueResult.CANCELLED_CREATE`)
- 起動時に前回送れなかった分を送り直す(`resendPending`)。止まった列は `RETRY_EVERY_MILLIS`(30 秒)ごとと、ウィンドウが前に出た時(`resume`)に起こす
- 送って断られた時は、端末の行が戻るので、その行だけ画面に混ぜ直す(`Reload`・`onReload`)
- 送り待ちの件数(そのアプリの分)は `pending` で画面に出す

## 消す時: 消える物と残る物

コマンドパレットの「この端末のデータを消す」は `AppHub.wipeLocalData()`(`Windows.kt`)から次の順で動く。サインアウトの画面(#1749)ができたら同じ範囲を使う想定(`DesktopStore.wipe` のコメント)。

1. 開いている全アプリの書き換えを止め、積んだ書き込みを書き終えるのを待つ
2. 各アプリの `SessionPersistence.wipe()` → `DesktopStore.wipe()`
3. `DeviceWipe.run(root)`(`Main.kt` の `wipeStore`)
4. 写真を「既定のアプリで開く」時に書いた一時ファイルを消す(`clearTempFiles` = `PhotoOpen.clearTemp`)

| 物 | 消える / 残る | どこで |
|---|---|---|
| 記録(`records/`) | 消える | `RecordStore.removeEverything` |
| 送信待ち(`queue/`) | 消える | `SyncQueue.discardAll`(列を作っていなければフォルダごと) |
| DSL(`documents/`) | 消える | `DocumentCache.removeAll` |
| 設定(`settings.json`) | 消える | `JsonKeyValueStore.clear` |
| AI への送信の同意 | 消える(人ごとの物) | `AiConsentStore.forget` |
| クラッシュ報告の設定 | 残る(端末ごとの物) | `consent.json` の他の鍵は消さない |
| `*.tmp`・`*.corrupt` | 消える | `sweepTempFiles`・`DeviceWipe.sweepStray` |
| 貼った写真・引き出す一時ファイル・カレンダーに渡したファイル | 中身が消える(フォルダは残す) | `DeviceWipe.emptyDir` |
| 予約した通知・覚えた画像 | 消える | `Notifier.Tray.cancelAll`・`ImageLoader.clear` |
| クリップボード | 書き出した表のままなら空にする | `ExportClipboard.clearIfOurs` |
| 音声のモデル(`models/`) | 残る(個人の情報ではなく、落とし直すと約 60MB) | `VoiceModels` のコメント |
| ウィンドウの大きさ・位置 | 残る | `java.util.prefs` は触らない |

## 決めること

- サインアウトの画面(#1749)で、上と同じ範囲を消すか(コードのコメントは同じ範囲の想定。画面はまだ無い)
- `queue/queue.json` が読めない時も `settings.json` と同じく `.corrupt` によけるか(今は空として読む。送っていない書き込みの扱いに関わる)
