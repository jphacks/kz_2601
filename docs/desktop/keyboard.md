# デスクトップ版のキーボード操作

デスクトップ版(`desktop/`)をキーボードだけで使うためのショートカット・コマンドパレット・Tab の順(#1765)。コードは `desktop/app/src/main/kotlin/jp/kz2601/metamon/desktop/` の `Keyboard.kt`・`CommandPalette.kt`、テストは `desktop/app/src/test/kotlin/jp/kz2601/metamon/desktop/KeyboardTest.kt`。全体は [`desktop/README.md`](../../desktop/README.md)。

## 目次

- [ショートカットの一覧](#ショートカットの一覧)
- [OS ごとの決まり](#os-ごとの決まり)
- [入力欄との取り合い](#入力欄との取り合い)
- [Esc と Delete](#esc-と-delete)
- [コマンドパレット](#コマンドパレット)
- [Tab の順](#tab-の順)
- [表をコードと照らし合わせる](#表をコードと照らし合わせる)

## ショートカットの一覧

F1 の一覧(`ShortcutHelp`)と同じ並び(`Shortcut` の並び)。組みが 2 つある物は ` / ` で区切り、前の方をパレットのヒントに出す。

| 操作 | Windows・Linux | macOS |
|---|---|---|
| コマンドパレット | Ctrl+K / Ctrl+Shift+P | ⌘K / ⇧⌘P |
| 新規 | Ctrl+N | ⌘N |
| 保存 | Ctrl+S | ⌘S |
| 検索 | Ctrl+F | ⌘F |
| 削除 | Delete | ⌘⌫ / ⌦ |
| 閉じる / 戻る | Esc | Esc |
| 印刷 | Ctrl+P | ⌘P |
| ショートカット一覧 | F1 / Ctrl+/ | F1 / ⌘/ |
| 写真を大きく見る | Space | Space |

- 操作の名前は文言のキー `desktop_keys_*`(印刷は `desktop_print`)。英語の OS では英語で出る
- 印刷は `Keymap` にあるが、押した時の処理は `Print.kt`(`isPrintKey`)。印刷できる画面が無い DSL では受けない
- コマンドパレットを Ctrl+P にしないのは印刷とぶつかるため(`Keymap` のコメント)
- 写真を開いている間は Esc・Space で閉じ、← → で前後の写真に送る(`PhotoPreview.keyOf`)

## OS ごとの決まり

`HostOs.of(os.name)` で分ける(`mac`・`darwin` → macOS、`windows` → Windows、他は全部 Linux)。

- Windows・Linux は Ctrl、macOS は ⌘ を使う(`KeyChord.primary`)
- 使わない方の修飾キー(Windows・Linux の ⌘ / Win、macOS の Ctrl)が押されていたら取らない。macOS の Ctrl+N はカーソル移動なので取らない
- Alt(Option)が付いていたら取らない
- Shift は組みと同じ時だけ取る(Shift+Delete は切り取りなので削除にしない)
- 表記(`Keymap.label`): Windows・Linux は `Ctrl+Shift+P`、macOS は `⇧⌘P`。Delete は macOS で `⌦`、Backspace は `⌫`
- macOS の削除は ⌘⌫(Finder と同じ)。fn+delete(⌦)も受ける。Backspace だけでは削除にしない(文字を消すだけ)

## 入力欄との取り合い

`DesktopKeyboard.modifier` をウィンドウの一番外に付け、2 回受ける。

| 受ける時 | ショートカット | 意味 |
|---|---|---|
| 入力欄より先(`onPreviewKeyEvent`) | パレット・新規・保存・検索 | 入力欄にいても効く |
| 入力欄の後(`onKeyEvent`・`Shortcut.bubble`) | 削除・閉じる / 戻る・一覧・写真 | 入力欄が使えばそちらが先(Delete は文字を消す、Space は空白を入れる) |

- パレット・一覧・ライセンス・消す前の確認を出している間は、それぞれが自分でキーを受ける
- Space で写真を開くのは、何も選んでいない(一番外がフォーカスを持つ)時だけ。押しっぱなしの 2 回目以降は開閉しない
- 新規・保存が画面に無い時は知らせを出す(「この画面には新規がありません」など)。検索欄が無い画面の Ctrl+F はパレットを開く

## Esc と Delete

- Esc: 閉じられる画面(sheet・push した画面など。`session.canClose`)なら閉じて戻る。一番下の画面では取らず、他へ流す
- Delete: 画面に見えている削除ボタン(toolbar → 部品の順で最初の物)を押す。確認の文が無いボタンには「削除しますか？」(`desktop_keys_delete_confirm`)の確認を付ける(`ScreenShortcuts.withConfirm`)。見えている削除ボタンが無ければ取らない
- パレットから削除ボタンを選んだ時も同じ確認を付ける

## コマンドパレット

Ctrl+K で開く(`CommandPalette`)。

- 操作: 文字で絞り込み、↑ ↓ で選び(端で反対側へ回る)、Enter で実行、Esc か外側のクリックで閉じる
- 中身の並び(`DesktopKeyboard.commands`): ショートカット(パレット自身を除く・キーのヒント付き)→ この画面のボタン(見えている物)→ 今の記録を別のウィンドウで開く → 画面 → 同梱のアプリ(別のウィンドウで開く)→ この端末のデータを消す(確認あり)→ ライセンス
- 絞り込み(`CommandRanking`): 全角半角・大文字小文字・カタカナとひらがなを同じに見る。点は 完全一致 > 頭から一致 > 語の頭から一致 > 途中に含む(前ほど上)> 別名(画面の id など)> 文字が順に出てくる。同じ点は元の並び

## Tab の順

- Tab / Shift+Tab で部品を移る。グリッドは行 → 列の順(F1 の一覧の下の一言 `desktop_keys_help_tab`)。Enter / Space で押す
- 推測: 並びを決める専用のコードは無く、Compose の既定の順(描いた順)に従っている。グリッドを行ごとに描いているので行 → 列になる
- sheet・fullScreen を開いている間は下の画面に Tab で入らず、読み上げにも出さない(`DesktopKeyboard.inert`)。開いた時は中の最初の部品へ入る(`modalModifier`・`focusAfterNavigation`)
- 写真のプレビューを開いている間も下の画面には入らない

## 表をコードと照らし合わせる

上の表を直す時・`Keyboard.kt` を直した時は次で照らし合わせる。

1. 行の並び: `Keyboard.kt` の `enum class Shortcut` の並び
2. キー: `Keymap.chords` の `when` の各行。`primary = true` は Ctrl / ⌘、`shift = true` は Shift
3. 表記: `Keymap.label` と `keyName`。例は `KeyboardTest.labelsFollowPlatform`
4. 手元では、アプリを開いて F1 を押し、一覧と表を並べて見る(macOS の表記は Mac で開いた時だけ出る)

```sh
cd desktop
./gradlew :desktop:test --tests '*KeyboardTest*'
```
