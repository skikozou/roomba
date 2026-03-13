# Roomba 980 調査ノート（2026-03-11）

## 目的
- Roomba 980 を「どこまでいじれるか」「何をいじれるか」「誰がいじった実績があるか」を、公式情報と主要コミュニティ実装を中心に整理する。
- `roomba` リポジトリで現実的に実装できる制御範囲を明確化する。

## 結論サマリ
- **一番実用的な制御面は LAN 内のローカル MQTT/TLS API**（BLID + password + robot IP）です。公式アプリ相当の主要操作（開始/停止/一時停止/ドック復帰/状態取得）は実績が多いです。[3][6][7][8][9]
- **Roomba 900 系（980含む）は 2.4GHz 前提**。セットアップ時のネットワーク条件（SSID, discovery, firewall port）が非常に重要です。[2]
- **ローカル接続は1クライアント制約が実運用上のボトルネック**。同時にアプリや他ツールが繋がると競合しやすいです。[3][4][7]
- **ハード改造は「修理・基板交換」系の情報は多い**が、980向けの公式公開低レベル制御仕様（シリアルOI）は確認しづらく、ソフト制御は実質 MQTT 系が主流です（推論）。[1][5][6][9]

---

## 1. 公式情報で確認できる範囲

### 1.1 製品の位置づけ（2015年リリース）
- iRobot公式発表では、Roomba 980 は「iAdapt 2.0 + Visual Localization (vSLAM)」「Wi-Fi / アプリ制御」「再充電して続きから清掃」が強調されています。[1]
- 2015-09-17（米加）発売、当時価格 $899 と明記されています。[1]

### 1.2 Wi-Fi要件・ネットワーク条件
- iRobotサポート記事で、**Roomba 900 は 2.4GHz 対応、5GHz 非対応**と示されています。[2]
- 同記事に、接続時の推奨ポートとして以下が記載されています。[2]
  - Discovery: UDP 5353/5678
  - Data: TCP/HTTPS 443, TCP/MQTT 8080/8883
  - Outbound: UDP 123 (SNTP), UDP/TCP 53 (DNS)
- メッシュ環境やSSID分離時の注意（2.4GHz側を選ぶ必要）が具体的に説明されています。[2]

### 1.3 取扱説明書（900シリーズ）で分かること
- 900シリーズ Owner’s Guide は 960/980 が対象。[10]
- 本体要素として iAdapt localization camera, RCON sensor, Wi-Fi indicator などが記載。[11]
- iRobot HOME App でスケジュール（週最大7回）、清掃設定、ソフトウェア更新を扱う旨が記載。[11]

---

## 2. 実際に「いじれる」層

### 2.1 アプリ/クラウド層（公式UX）
- 公式経路は iRobot HOME App 中心で、スケジュール・開始/停止・設定変更などを提供。[1][11]
- ただし細かい内部プロトコルは公開されておらず、外部連携はコミュニティ実装依存が実態。

### 2.2 ローカル API 層（コミュニティの主戦場）
- `dorita980` は「古いWi-Fi Roombaの local MQTT/TLS protocol」を対象にし、900系（980含む）を明示サポート。[6]
- 認証は BLID/Password（クラウド取得・ローカル取得の手順あり）。[6][3]
- 主要メソッド実装例:
  - `start/pause/stop/resume/dock/find`
  - `getRobotState`, `getMission`, `getPreferences`, `getWirelessStatus`
  - `cleanRoom(args)`（機種依存）
  - `setCarpetBoost*`, `setEdgeClean*`, `setCleaningPasses*`, `setAlwaysFinish*`
  - `state` event で状態ストリーム購読
  [6]

### 2.3 連携ラッパー層（運用しやすい形）
- `rest980`: dorita980 を HTTP API 化。`/api/local/action/*`, `/api/local/info/*`, `/api/local/config/*` で操作可能。[7]
- Home Assistant `roomba` integration:
  - ローカル接続で使う構成（IoT class: Local Push）
  - credential取得手順（BLID/password）
  - Roomba MQTT が single connection 制約である点を明記
  [3]
- openHAB iRobot binding:
  - ローカル直結（専用MQTTブローカ不要）
  - command/phase/battery/bin/error/rssi/snr/schedule 等のチャネル
  - single local connection 制約と再起動回避策の言及
  [4]
- `roombapy`（HA向けフォーク）は firmware 2.x.x + local only を明示。[8]

### 2.4 ハードウェア層（修理/分解）
- iFixit の 980 マザーボード交換ガイドで、分解手順・コネクタ取り扱い注意・部品交換の具体手順が公開されています。[5]
- ＝「物理的に開けて修理・交換する」レベルは既にコミュニティ知見がある。

---

## 3. 何をいじれるか（実務観点）

### 3.1 高確度で可能（既存実装多数）
- 清掃制御: `start/stop/pause/resume/dock/find`
- 清掃状態取得: phase, cycle, error, battery, bin, mission counters
- Wi-Fi/本体情報取得: wireless status, software version, sku 等
- 清掃設定変更: carpet boost, edge clean, pass数, always finish, schedule
- イベント購読: MQTTベースで状態更新を継続受信
  [3][4][6][7][8][9]

### 3.2 条件付きで可能（機種/FW依存）
- 部屋/領域清掃（`cleanRoom` / `cleanRegions`）
  - 980で必ず同等に使えるとは限らず、マップ機能の世代差・FW差が絡む。
  - i7/s9系での実績記述が多く、980では要実機検証。
  [4][6][7]

### 3.3 難しい/不確実
- 非公開低レイヤ（ファーム書換え、ブートローダ、隠しI/F）
  - 公開情報だけで再現可能な手順は乏しい。
- 公式サポート外の深い改造
  - 破損・復旧不能リスク、保証喪失リスクが高い。

---

## 4. 制約・ハマりどころ

- **Single local connection**
  - ローカルMQTT接続は同時多重に弱く、アプリ/HA/openHAB/独自ツールの競合が起きる。[3][4][6]
- **FW差分と互換性**
  - 900系はv2系を前提にした実装が多い。ライブラリによって対象FWが異なる。[6][8][9]
- **OTA更新で挙動変化**
  - dorita980作者は「更新で互換性が崩れることがある」と注意喚起。[6]
- **Wi-Fi要件依存**
  - 2.4GHz要件、ルータ設定、ポート/Discovery条件で接続可否が大きく変わる。[2]

---

## 5. 「誰がいじったか」実績（主要プロジェクト）

- `koalazak/dorita980`（Node SDK, local/cloud）[6]
- `koalazak/rest980`（RESTラッパ）[7]
- `NickWaterton/Roomba980-Python`（Python実装、状態解析/マップ系）[9]
- `pschmitt/roombapy`（HA向けPython実装）[8]
- Home Assistant公式 integration docs（運用知見）[3]
- openHAB iRobot binding docs（運用知見＋チャネル設計）[4]
- iFixit repair guides（ハード修理実績）[5]

---

## 6. このリポジトリ（`roomba`）で現実的な実装範囲

### 6.1 まず実装価値が高い
- ローカル接続確立（BLID/password/IP）
- コマンド最小集合: `start/stop/pause/resume/dock/find`
- 状態購読: battery/bin/error/phase/mission
- 接続排他制御（single connectionを前提に、再接続とバックオフ）

### 6.2 次段で有効
- 設定系: pass数, edge clean, carpet boost, schedule
- ネットワーク診断: 2.4GHz/ポート/同時接続競合のセルフチェック
- room/region clean（980実機での可否を feature flag 化）

### 6.3 実装時の注意
- 「使える機能」をモデル/FW別 capability として管理する。
- app/他連携と同時利用するなら、接続所有権（誰が今MQTTを握るか）をUIで可視化する。
- センサー欠損時は即失敗ではなく、リトライ + stale扱い + 最終受信時刻表示にする。

---

## 7. 未確認事項（追加調査候補）

- 980での room/region clean の安定可否（FW個体差）
- 980の非公開ハードデバッグI/F（再現性ある公開手順の不足）
- 最新iRobotアプリとのセットアップ相性問題（端末OS/アプリ版依存）

---

## 8. 参考リンク

### 公式（一次情報）
1. iRobot Press Release (2015-09-16): https://media.irobot.com/2015-09-16-iRobot-Enters-the-Smart-Home-with-Roomba-980-Vacuum-Cleaning-Robot
2. iRobot Support (Wi-Fi setup / 2.4GHz / ports): https://homesupport.irobot.com/articles/en_US/Knowledge/17734
3. Home Assistant Roomba integration: https://www.home-assistant.io/integrations/roomba/
4. openHAB iRobot binding: https://www.openhab.org/addons/bindings/irobot/
5. iFixit Roomba 980 motherboard replacement: https://www.ifixit.com/Guide/iRobot+Roomba+980+Motherboard+Replacement/100170
10. iRobot Owner’s Guide index (Roomba 900適用明記): https://homesupport.irobot.com/articles/en_US/Knowledge/844
11. Roomba 900 Owner’s Guide (EN-US PDF): https://prod-help-content.care.irobotapi.com/files/hs/roomba/900/manual/en-US.pdf

### コミュニティ実装（一次情報）
6. dorita980: https://github.com/koalazak/dorita980
7. rest980: https://github.com/koalazak/rest980
8. roombapy: https://github.com/pschmitt/roombapy
9. Roomba980-Python: https://github.com/NickWaterton/Roomba980-Python

