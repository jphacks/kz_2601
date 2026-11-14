# App Store Connect の「App のプライバシー」を直す手順(2026-11-08 の改定)

プライバシーポリシーの 2026-11-08 の改定に合わせて、App Store Connect(ASC)の「App のプライバシー」の答えを直す手順です。**人の作業**です(エージェントは ASC を触らない)。答えの根拠は [privacy-label.md](privacy-label.md)。

## 目次

1. [何が変わるか](#1-何が変わるか)
2. [今の審査用の版のために、今すぐ直す](#2-今の審査用の版のために今すぐ直す)
3. [次のビルドで確かめる](#3-次のビルドで確かめる)
4. [通常の版を App Store に出す時に足す](#4-通常の版を-app-store-に出す時に足す)
5. [ほかに直す所](#5-ほかに直す所)

## 1. 何が変わるか

| 種類(ASC の表記) | 今の答え | 新しい答え | 理由 |
|---|---|---|---|
| Usage Data > Product Interaction(使用状況データ > 製品の操作) | 無し | **足す**。用途 Analytics・Linked to the user: Yes・Used for tracking: No | サーバーの利用の集計(ポリシー 1.12)。`uid_hash` 付きで、**全部の版(審査用の版も)が対象** |
| Identifiers > User ID(ID > ユーザー ID) | App Functionality | **App Functionality と Analytics** | 同じ集計で uid から作った値を使う |
| Data Used to Track You | 無し | 無しのまま | 広告 ID・トラッキングは使わない |
| ほかの種類 | — | 変えない | aiChat・長い答え・写真のボタンは、宣言済みの種類(その他のユーザーコンテンツ・写真またはビデオ)の中。地図(MapKit)は端末から Apple に直接で、運営者は受け取らない |

## 2. 今の審査用の版のために、今すぐ直す

サーバーの集計は今のビルドのままでも動いているので、ビルドを待たずに直す。

1. ASC > アプリ「Tailor It」> **App のプライバシー**(App Privacy)を開き、「データの種類」の「編集」を押す。
2. **Usage Data > Product Interaction** にチェックを入れて保存する。
3. Product Interaction の設定で:
   - 用途: **Analytics** だけを選ぶ。
   - 「ユーザーに紐づけていますか」: **はい**(Yes)。
   - 「トラッキングに使いますか」: **いいえ**(No)。
4. **Identifiers > User ID** の設定を開き、用途に **Analytics** を足す(App Functionality は残す)。紐づく: はい・トラッキング: いいえのまま。
5. プライバシーポリシーの URL が `https://tailor-it.app/privacy` のままか確かめる。**先に tailor-it.app のポリシー(5 章)を新しい文に差し替えてから**次へ進む。
6. 画面右上の **公開**(Publish)を押す。App のプライバシーの答えは、新しい版を出さなくても公開できる([Apple の案内](https://developer.apple.com/app-store/app-privacy-details/)。画面の表示に従う)。
7. App Store のページの「App のプライバシー」に「製品の操作」が出たことを、公開の後に確かめる。

## 3. 次のビルドで確かめる

この PR で `ios/Metamon/PrivacyInfo.xcprivacy` に Product Interaction(Analytics)と、User ID の Analytics を足した。次にアップロードするビルドで:

1. Xcode > Organizer > アーカイブを選ぶ > **Generate Privacy Report**。
2. 報告に、Product Interaction(Analytics・Linked)と User ID(App Functionality・Analytics)が出て、2 章で ASC に入れた答えと食い違いが無いか見る。
3. アップロードの後、Apple からプライバシーの警告のメール(ITMS-91053 など)が来ていないか見る。
4. 新しいビルドは同意の版が 3 に上がっている。以前に同意した人には、起動時に 1 回だけ同意の画面が出直す(審査用の版でも出る)。審査メモに書く必要は無い見込み。

## 4. 通常の版を App Store に出す時に足す

今は審査用の版だけを出している。通常の版(課金・フィードバック・プッシュ・オンラインの AI がある版)を出す時は、[privacy-label.md](privacy-label.md) の 4 章の「増える種類」も足す:

| 種類(ASC の表記) | 用途 | 紐づく | トラッキング |
|---|---|---|---|
| Purchases > Purchase History | App Functionality | Yes | No |
| User Content > Customer Support(フィードバック) | App Functionality | Yes | No |
| Identifiers > Device ID(プッシュの端末のトークン) | App Functionality | Yes | No |
| Diagnostics > Other Diagnostic Data(フィードバックの版・OS・機種) | App Functionality | Yes | No(宣言済み) |
| User Content > Photos or Videos(フィードバックのスクショ・写真のボタン) | App Functionality | Yes | No(宣言済み) |

- 写真のボタン(処方箋・薬の袋)を出すなら、**Health & Fitness > Health** を申告するかを先に決める(privacy-label.md 5 章)。
- 製品の操作は 2 章で足し済み(通常の版の aiButton の集計も同じ種類)。

## 5. ほかに直す所

- **tailor-it.app のポリシー**: 公開中のポリシーは別の repo(2410as/tailor-it-web)にある。`docs/legal/privacy-policy.md` の新しい文(日本語・英語・改定日 2026年11月8日)を写す。ASC の公開(2 章 6)より先か同時に。
- **写真のボタン**: サーバーの `AI_TEXT_IMAGE` は既定で off。このポリシーが公開され、同意の版 3 の iOS が行き渡ってから on にする(人の判断)。
