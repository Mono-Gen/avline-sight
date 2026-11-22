# AVLine-Sight

NETGEAR AV Line（M4250シリーズ）で組んだ AV over IP ネットワーク向けの専用監視ソフトです（Dante / AES67 / NDI / SDVoE / ST 2110 / Q-SYS / sACN / AVB）。
WebUIを同梱したWindows用の単一実行ファイルで、データはすべて実行ファイルと同じ階層の `./data/` に保存します。そのままコピーして持ち運べます。

English: [README.md](README.md)

## 主な機能
- SNMPv2cを3段階周期（Fast / Medium / Slow）でポーリングします。スイッチごとに独立したワーカーで動くため、1台が応答しなくても他のスイッチは遅れません。応答しないスイッチは確認間隔を 5秒 → 15秒 → 30秒 と延ばします。
- SNMP Trapを受信します（linkUp/Down・LLDP・PoE・ファン/電源/温度・STP・IP重複・CPU）。未定義のTrapも生のOIDのまま保存します。
- 筐体ヘルス：CPU（5秒/60秒/300秒）、プロセス別CPU、メモリ、Mbuf、温度、ファン、電源、再起動検知（sysUpTimeの巻き戻り）。
- ポートと接続機器：LLDP・DHCP Snooping・ARP・FDB＋OUIベンダー辞書・手動ラベルで機器を特定し、接続の履歴を年表で表示します。
- AV向けチェック：IGMPクエリアの状態、マルチキャストのグループ×ポート表、EEE/LPI、PAUSEフレーム（NDI4は例外として扱います）、CoSキューのドロップ、DSCP Trust、FrameTooLongs（MTU不一致）、UDLD、SFP DOM、PoE障害、STP/MSTP、二重DHCP・不正DHCPサーバー。
- NETGEAR ProAVガイドに照らした全スイッチ横断の監査：FWバージョン不一致、STP優先度、クエリアの配置、Snooping設定の不一致、アップリンクのMTU、デイジーチェーンの段数。
- リハーサル時のベースライン保存と差分表示、インシデント・マーカー（キー `M`）、`.avsight` によるセッション書出とオフラインのタイムマシン再生、CSV/JSON出力。
- 内蔵Telnet/SSHターミナル（xterm.js）。`show` コマンドをワンクリックで送れます。接続先は登録済みのスイッチに限られ、認証情報は保存しません。10分間操作がなければ切断します。
- 内蔵の仮想M4250ラボ（本物のSNMPv2cエージェント、障害注入、模擬CLI）で、実機がなくても動作を確認できます。

## 使い方
```powershell
# 実機なしで試す（仮想M4250×4台）
.\avline-sight.exe --lab 4 --open
```
```powershell
# 本番運用（WebUI は localhost:8080、Trap は UDP 162 で待ち受け）
.\avline-sight.exe --open
```
UDP 162 で待ち受けできない場合も、アプリは止まらず画面に警告を出します。原因は、Windows標準の「SNMP Trap」サービスがポートを使用中か、権限が足りないかのどちらかです。待受ポートは画面か `--trap` で変更してください（例 `0.0.0.0:1162`）。

| オプション | 既定値 | 内容 |
|---|---|---|
| `--listen` | `127.0.0.1:8080` | WebUIの待受アドレス。LANに公開する場合は `0.0.0.0:8080` とし、閲覧専用ロックと併用してください。ポートが使用中なら空いている次のポートを使います |
| `--trap` | `0.0.0.0:162` | SNMP Trapの待受アドレス |
| `--data` | `<exeの場所>/data` | DBと設定の保存先 |
| `--lab N` | `0` | 仮想スイッチをN台（1〜20）起動して監視します。ラボは本番とは別のDBを使います |
| `--fast/--medium/--slow` | `3s/12s/45s` | ポーリング周期 |

## SNMP Trap の受信を確認する

Trapを受けると、障害にすぐ気づけて、スイッチへの問い合わせも減らせます。受信できていなくても監視は続きますが、一瞬のリンク断などは見逃しやすくなります。

**1. アプリの表示を見る**: 画面上部のTrap受信の状態が「待受中」なら受信できる状態です。「待受できません」なら下の2へ進みます。スイッチごとの「最終Trap受信」が空のままなら、スイッチ側の送信設定を確認してください。

**2. 162番を使っているものを調べる**（PowerShell）
```powershell
Get-NetUDPEndpoint -LocalPort 162 -ErrorAction SilentlyContinue | Select-Object LocalAddress, OwningProcess
Get-Service SNMPTRAP -ErrorAction SilentlyContinue
```
- 何も表示されなければ、162番は空いています。管理者権限が必要な環境では、アプリを「管理者として実行」してください。
- `SNMPTRAP`（Windows標準の「SNMP Trap」サービス）が Running なら、それが162番を使っています。

**3. 使えないときの対処**（どれか1つ）
- Windowsの「SNMP Trap」サービスを止める（管理者のPowerShell）: `Stop-Service SNMPTRAP; Set-Service SNMPTRAP -StartupType Disabled`
- 別のポートで待ち受ける: `--trap 0.0.0.0:1162` で起動し、スイッチ側の送信先ポートも 1162 にする。
- Windowsファイアウォールで受信を許可する（管理者）: `New-NetFirewallRule -DisplayName "AVLine-Sight Trap" -Direction Inbound -Protocol UDP -LocalPort 162 -Action Allow`（ポートを変えた場合はその番号）

**4. スイッチ側の送信設定**: 例は `docs/api.md` を参照してください（実機での確認はまだです）。

## 複数のPCで見る

監視するアプリ（exe）は1台だけで動かし、ほかのPCはブラウザで `http://<そのPCのIP>:8080` を開いてください（`--listen 0.0.0.0:8080` で起動し、閲覧専用ロックを併用）。確認・運用フェーズ・作業中・ロックはすべてのPCにすぐ反映されます。アプリを複数台で動かすと、スイッチへの問い合わせが倍になり、確認状態も共有されないためおすすめしません。

## ビルド
必要なもの: Go 1.26以上、Node.js 20以上。
```powershell
pwsh scripts/build.ps1 -Version 1.0.0
```
WebUI（`web/dist`）のビルド、Goのテスト実行、`bin/avline-sight.exe` の生成までを一度に行います。

## 資料
- API仕様: [docs/api.md](docs/api.md)
- 対応MIB: NETGEAR M4250 FW 13.0.7.6 のMIB。`internal/mib` のOIDはすべてMIBファイルと機械的に照合済みです。

## 既知の制約
- 動作確認は内蔵シミュレータでのみ行っており、実機のM4250ではまだ試していません。実機に初めてつなぐときは「プローブ」を実行し、診断ダンプで機種ごとの違いを確認してください。たとえば SFP DOM の単位（`DBmHundreths`、現在は÷100で換算）です。
- OUI辞書には主要ベンダーしか収録していません。QSC・Biamp・MA Lighting・Clear-Com・BirdDog は未収録です。
- SSHのホスト鍵は固定（ピン留め）しません。接続時に指紋を表示するので、目で照合してください。
- SNMPv3・外部通知・sFlow・SNMP SETによる操作は v2 で対応予定です。
