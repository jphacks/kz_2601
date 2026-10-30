# #426 端末内の LLM を必要な時だけ入れる(後からダウンロードする形)

> 状態: **提案(オーナーの判断待ち)**。2026-10-28 作成。
> 対象 Issue: [#426](https://github.com/jphacks/kz_2601/issues/426)
> 調べた日: 2026-10-28。OS の版・規約は変わるので、決める前にもう一度「参考」のページを見ること。

## 目次

1. [結論(推奨)](#結論推奨)
2. [選択肢の表](#選択肢の表)
3. [根拠](#根拠)
4. [決めてほしいこと](#決めてほしいこと)
5. [参考](#参考)

## 結論(推奨)

**M0 では今の Foundation Models(`aiButton`・#78)のままにし、別のモデルのダウンロードは入れない。** 足りない用途が実際に出るか(非対応端末の利用者の割合・`aiButton` が答えを出せない依頼)を M2 の β で測ってから決める。

- 次の一手は、モデルを配ることより **iOS 27 の Private Cloud Compute(PCC)版の Foundation Models** を試す方が安い見込み。同じ API のまま、文脈が 4K → 32K トークンになり、推論(reasoning)も使える。ただし使うには Apple の**利用資格の申請(managed entitlement)が要り、条件は要確認**。対応端末は Apple Intelligence 対応機だけで、非対応端末の問題は解けない。
- 非対応端末(今の検証機 iPhone 13 を含む)でも端末内の AI を使いたい、となった時だけ、**MLX Swift(MIT)+ Apache-2.0 の小さいモデルを「データ」として後からダウンロード**する形にする。配り方は Background Assets(Apple が配信)を第一候補、WhisperKit と同じ自前ダウンロードを第二候補にする。
- どの形でも、**実行するコード(推論の処理・部品・計算の種類)はバイナリに入れて審査を通す**。後から入れるのはモデルの重み(数値のデータ)だけにし、重みを入れ替えても使える機能(`aiButton` の task の種類)は増やさない(2.5.2)。

## 選択肢の表

| 選択肢 | 意味 | 費用 | 審査 | 作業量 |
|---|---|---|---|---|
| **A. Foundation Models だけ(推奨・M0)** | 今の `aiButton`。OS に入っているモデルを使う。非対応端末では理由付きで無効 | 0(端末内) | 論点なし(実装・審査済みの形) | 0 |
| B. A + PCC(iOS 27〜) | 長い文・複雑な依頼だけ PCC に回す。ネットが無い・上限に達したら端末内に戻す | Tailor It の費用は 0 の見込み(利用者ごとに 1 日の上限があり、超えると iCloud+ の案内が出る) | Apple の資格の申請が要る(**条件は要確認**)。データが端末外(PCC)に出るので、プライバシーの説明(「データは端末外に出ない」の文言)を直す必要あり | 小〜中(availability の分岐・上限の UI・文言) |
| C. A + 小さいモデルを後から DL(Background Assets・Apple が配信) | 非対応端末向けに、MLX Swift + 1〜3B 程度の量子化モデルを、使う時だけ入れる | 配信は Apple(App Store Connect にアセットパックを上げる)。容量の上限はアプリ全体で 200 GB・200 パック | 重みはデータとして配る。推論コードはバイナリ。**ダウンロードで機能が増えると読まれない設計が要る**(2.5.2) | 大(推論の組み込み・メモリ対策・品質評価・iOS 26 以降の API) |
| D. A + 小さいモデルを後から DL(自前の配信) | C と同じだが、配信は Hugging Face や Cloud Storage から URLSession で落とす(WhisperKit と同じ形) | 配信の転送量(自前なら Cloud Storage の外向き転送。**要確認**) | C と同じ | 大(C + 再開・検証・容量不足の扱い) |
| E. On-Demand Resources で配る | 昔からある Apple の仕組み | Apple が配信 | **iOS 27 で非推奨**(将来の版で削除予定と Apple が案内) | 中。ただし今から使う理由が無い |
| F. クラウドの LLM(Claude 等)を生成アプリの中から呼ぶ | 端末内をやめて backend 経由で呼ぶ | 利用量に比例(`docs/backend/rate-limit.md` の上限の仕組みに乗せる) | 4.7(チャットボット)・プライバシーの表示の見直し | 中(Issue の趣旨からは外れる。参考として載せる) |

## 根拠

### 1. Foundation Models(OS のモデル)

- Apple Intelligence の端末内のモデルを使う枠組み。要約・抽出・分類・構造化出力(guided generation)・道具呼び出しができる(Apple の文書)。
- **対応端末**(Apple のサポートページ・2026-10-28): iPhone 15 Pro / 15 Pro Max、iPhone 16 シリーズ以降、iPhone Air、iPhone Duo(ページの記載のまま)。**iPhone 13・14・15(無印)は非対応**。日本語は対応言語に入っている。端末の空き容量が最大 8 GB(17 Pro 以降・Air・Duo は最大 14 GB)要る。
- モデルは OS の更新で入れ替わる。今は 3 つの版(iOS 26.0〜26.3 / 26.4 / 27.0)がある。版が変わると答えが変わりうるので、`aiButton` の出力は「下書き」に留める今の設計が合っている。
- 端末内のモデルの文脈は約 4K トークン(`docs/ios/architecture.md` §18 で入力を 3000 文字に制限済み)。
- **iOS 27 の PCC 版**(`PrivateCloudComputeLanguageModel`): 32K トークン・推論あり・ネットが要る・利用者ごとに 1 日の上限あり(上限を超えたら iCloud+ への案内を出す仕組みが用意されている)。API キーや認証の管理は要らない。利用には資格の要件があり、managed entitlement を申請する(**条件は要確認**)。

### 2. 後からの配り方と審査 2.5.2

- **2.5.2(本文)**: アプリは自分のバンドルで完結し、**アプリの機能を加えたり変えたりするコードをダウンロード・インストール・実行してはいけない**。
- モデルの重みは数値のデータで、それ自体は実行コードではない、という読み方で WhisperKit の音声モデル(`small`・約 500 MB を初回に自前でダウンロード)を既に配っている(`docs/ios/architecture.md`)。同じ形なら通る見込みが高いが、**LLM の重みで同じ判断になるかは Apple の明文が無く、要確認**(審査メモに「推論コードはバイナリに入っている・重みはデータ・重みで部品や機能は増えない」と書いておく)。
- 守ること: 推論のランタイム(MLX Swift 等)はビルド時に入れる。重みと一緒に「プロンプトのテンプレート」「新しい task の種類」を配って機能を増やさない。Tailor It はもともと「部品と計算の種類はバイナリに固定・増えるのは更新の時だけ」と審査に説明している(`docs/store/app-review-notes.md`)ので、それと矛盾させない。
- **Background Assets**: システムが配信を管理し、Apple のサーバーに置くこともできる(Apple-Hosted)。アセットパックごとに「起動前に必須」「先読み」「必要な時だけ」を選べる。`AssetPackManager` は **iOS 26.0 以降**。Apple が配信する場合、アプリ全体で**合計 200 GB・200 パックまで**(パック 1 つの上限はページに書かれていない)。2026 年 6 月の App Store Connect のリリースノートに「大きなアセットパックの入れ込みが失敗することがある」既知の問題がある。Apple は「Background Assets は追加のアセットを落とす目的だけに使うこと」と書いている。
- **On-Demand Resources**: iOS 27 で非推奨、将来の版で削除予定(Apple の案内)。Background Assets への移行を勧めている。→ 使わない。
- 自前ダウンロード(案 D): 自由度は高いが、再開・ハッシュの検証・容量不足・モバイル回線での確認を自分で作る。WhisperKit の実装を参考にできる。

### 3. iPhone での大きさとメモリ

- 目安(**要実測**): 1〜2B パラメータを 4bit に量子化すると重みだけで約 0.6〜1.2 GB、3B で約 1.8 GB。動かすとさらに作業用のメモリ(KV キャッシュ等)が要る。
- iPhone の RAM の量を Apple は公表していない(**機種ごとの値は要確認**。一般に、Apple Intelligence 対応機は 8 GB 以上、iPhone 13 はそれより少ないとされる)。非対応端末ほどメモリが少ないので、「非対応端末のために配る」案は、一番動かしにくい端末を相手にすることになる。
- アプリが使えるメモリの上限は公表されておらず、超えると OS に終了させられる。`com.apple.developer.kernel.increased-memory-limit`(iOS 15 以降)で、対応端末では上限を上げてもらえる場合がある。**どれだけ上がるかは要実測**。
- WhisperKit(約 500 MB)と同時に読み込まない設計が要る(音声入力と `aiButton` は同時に使わないので、使う時だけ読み込む)。
- 端末の空き容量: Apple Intelligence 自体が最大 8〜14 GB を使う。さらに 1〜2 GB を足すことになる。

### 4. ランタイムとライセンス(2026-10-28 に GitHub と Hugging Face で確認)

| もの | ライセンス | メモ |
|---|---|---|
| MLX(`ml-explore/mlx`) | MIT | Apple の機械学習の枠組み。Apple シリコン向け |
| MLX Swift(`ml-explore/mlx-swift`)・例(`mlx-swift-examples`) | MIT | Swift から LLM を動かす例がある |
| llama.cpp(`ggml-org/llama.cpp`) | MIT | C/C++。GGUF 形式。Metal で動く |
| Qwen3-1.7B(例) | Apache-2.0 | 商用で使いやすい |
| Gemma 3 1B(例) | Gemma の利用規約(独自) | 利用規約への同意が要る(ダウンロードにゲートあり)。再配布の条件は**要確認** |
| Llama 3.2 1B(例) | Llama 3.2 Community License(独自) | 同上。表示の義務などの条件は**要確認** |

- ランタイムはどれも MIT なので、ライセンスの表示(謝辞)を入れれば使える。気をつけるのは**モデル側のライセンス**。Apache-2.0 か MIT のモデルを選ぶと、再配布・商用の条件が分かりやすい。
- 日本語の品質・速度・電池の減りは、どのモデルも**要実測**。

### 5. プロダクトの事情

- `aiButton` は「データが端末外に出ない・課金なし」を売りにしている(`docs/dsl/README.md`・`docs/ios/architecture.md` §18)。PCC(案 B)を入れるとこの説明が変わるので、文言とプライバシーの表示を見直す。
- 運転中のアプリで AI を使う場面(要約・分類)は短い文が中心で、端末内のモデル(4K)で足りる見込み。
- 非対応端末の利用者がどれだけいるかが分からない。β(M2)の計測(#295〜#298)で `aiButton` の availability の内訳を取ってから判断する方が、外れが少ない。

## 決めてほしいこと

| # | 決めること | 推奨 |
|---|---|---|
| 1 | M0 で別のモデルのダウンロードを入れるか | **入れない**(Foundation Models のまま) |
| 2 | 足りるかを測るか | **測る**: β で `aiButton` の availability の内訳(非対応端末・OFF・準備中)と、失敗した依頼を記録する(個人の文は送らない) |
| 3 | iOS 27 の PCC を試すか | **試す価値あり**。先に資格の申請の条件を調べる。入れるなら「データは端末外に出ない」の文言を直す |
| 4 | 非対応端末向けにモデルを配る場合の形 | MLX Swift(MIT)+ Apache-2.0 のモデル、Background Assets(iOS 26 以降)で配信。iOS 17〜25 向けは自前 DL |
| 5 | 審査メモにモデルの配り方を書くか | 配るなら**書く**(重みはデータ・推論コードはバイナリ・機能は増えない) |

## 参考

- [Foundation Models(Apple)](https://developer.apple.com/documentation/foundationmodels)
- [SystemLanguageModel(Apple)](https://developer.apple.com/documentation/foundationmodels/systemlanguagemodel)
- [Adding server-side intelligence with Private Cloud Compute(Apple)](https://developer.apple.com/documentation/foundationmodels/adding-server-side-intelligence-with-private-cloud-compute)
- [Apple Intelligence の対応端末と言語(Apple サポート)](https://support.apple.com/en-us/121115)
- [Background Assets(Apple)](https://developer.apple.com/documentation/backgroundassets)
- [Apple-hosted asset pack size limits(App Store Connect ヘルプ)](https://developer.apple.com/help/app-store-connect/reference/app-uploads/apple-hosted-asset-pack-size-limits/)
- [On-demand resources size limits(非推奨の案内)](https://developer.apple.com/help/app-store-connect/reference/app-uploads/on-demand-resources-size-limits)
- [increased-memory-limit entitlement(Apple)](https://developer.apple.com/documentation/bundleresources/entitlements/com.apple.developer.kernel.increased-memory-limit)
- [App Store Review Guidelines(2.5.2・4.7)](https://developer.apple.com/app-store/review/guidelines/)
- [MLX Swift](https://github.com/ml-explore/mlx-swift) / [mlx-swift-examples](https://github.com/ml-explore/mlx-swift-examples) / [llama.cpp](https://github.com/ggml-org/llama.cpp)
- [Qwen3-1.7B](https://huggingface.co/Qwen/Qwen3-1.7B) / [Gemma 3 1B](https://huggingface.co/google/gemma-3-1b-it) / [Llama 3.2 1B](https://huggingface.co/meta-llama/Llama-3.2-1B-Instruct)
- リポジトリ内: `docs/ios/architecture.md` §18(`aiButton`)、`docs/dsl/README.md`、`docs/store/app-review-notes.md`(2.5.2 の説明)
