# Roomba 980 Capability Matrix（2026-03-11）

Roomba 980を操作・監視する際の実装別比較メモ。

## 1. 実装別の制御・取得範囲

| 実装/資料 | 接続方式 | 主な操作 | 主な取得データ | 制約/注意 |
|---|---|---|---|---|
| iRobot HOME App（公式） | クラウド + ローカル連携 | 清掃開始/停止、スケジュール、設定変更 | ミッション状態、履歴、設定 | 内部APIは非公開、外部自動化は直接向かない |
| dorita980 | ローカル MQTT/TLS（+クラウド経由credential） | `start/pause/stop/resume/dock/find`, 設定系 (`setCarpetBoost*`等), `cleanRoom` | `getRobotState`, `getMission`, `getPreferences`, `getWirelessStatus`, state stream | 900系はFW v2系前提、機種/FW依存あり |
| rest980 | dorita980をREST化 | `/api/local/action/*`（start/stop/pause/resume/dock/find） | `/api/local/info/*`, `/api/local/config/*` | 元ライブラリ制約をそのまま継承 |
| Home Assistant Roomba | ローカル MQTT/TLS | start/pause/stop/return_to_base/locate | battery, bin, phase, state等（統合定義） | single local connection制約 |
| openHAB iRobot binding | ローカル（内部でMQTT使用）/クラウド | `command`チャネルで clean/spot/dock/pause/stop | `phase`, `battery`, `bin`, `error`, `rssi`, `snr`, schedule, pose | single local connection制約、接続競合注意 |
| Roomba980-Python | ローカル MQTT/TLS | clean/start/stop/pause/dock/find + region系（機種依存） | robot state, mission, pose, map関連（モデル依存） | 新しめFW/機種差の影響を受ける |

## 2. コマンド粒度の比較

| カテゴリ | 980で現実的 | 備考 |
|---|---|---|
| 基本清掃制御 | 高 | どの実装でも安定領域 |
| ドック復帰/探索音 | 高 | `dock/find` は実装実績が多い |
| 清掃設定（passes/edge/carpet） | 中〜高 | ライブラリ経由で可能、機種依存あり |
| room/region clean | 低〜中 | 980での安定性は要実機検証 |
| マップ詳細取得 | 低〜中 | 解析実装はあるがモデル/FW差が大きい |

## 3. テレメトリ項目（よく使うもの）

- バッテリ: `%`, 充電状態
- ミッション: `cycle`, `phase`, `error`, `sqft`, `mssnM`
- 本体状態: ビン状態、充電器接続、位置情報（pose系: x/y/theta）
- 無線: RSSI/SNR、接続品質
- 設定: edge clean, carpet boost, cleaning passes, always finish

## 4. 接続運用の落とし穴

- 2.4GHz必須（900系）。5GHzのみのSSIDだとセットアップ失敗。
- ルータ設定で discovery/通信ポートを塞ぐと初期接続が不安定。
- ローカル接続は1クライアント競合が起きやすい。
  - 例: アプリ起動中にHA/独自ツール接続で失敗
  - 対策: 接続所有権を1つに寄せる、再接続バックオフ実装、状態キャッシュを共有

## 5. `roomba` リポジトリ向け実装優先度

1. 接続管理の堅牢化（再接続、タイムアウト、排他）
2. コマンド最小セット（start/stop/pause/resume/dock/find）
3. センサー/状態取得の耐障害化（欠損時リトライ + stale表示）
4. capability判定（model/FW別に機能ON/OFF）
5. room/region等の拡張（feature flag）

## 6. 参照

- iRobot Wi-Fi setup/support: https://homesupport.irobot.com/articles/en_US/Knowledge/17734
- iRobot Roomba 900 owner guide PDF: https://prod-help-content.care.irobotapi.com/files/hs/roomba/900/manual/en-US.pdf
- dorita980: https://github.com/koalazak/dorita980
- rest980: https://github.com/koalazak/rest980
- Home Assistant Roomba integration: https://www.home-assistant.io/integrations/roomba/
- openHAB iRobot binding: https://www.openhab.org/addons/bindings/irobot/
- Roomba980-Python: https://github.com/NickWaterton/Roomba980-Python
- roombapy: https://github.com/pschmitt/roombapy

