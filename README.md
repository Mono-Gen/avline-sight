# AVLine-Sight

A dedicated monitoring tool for NETGEAR AV Line (M4250 series) networks used for AV over IP
(Dante / AES67 / NDI / SDVoE / ST 2110 / Q-SYS / sACN / AVB).
It ships as one portable Windows executable with the web UI built in, and stores everything in `./data/` next to the executable.

日本語版: [README_JP.md](README_JP.md)

## Features
- SNMPv2c polling in three tiers (Fast / Medium / Slow). Each switch has its own worker, so one dead switch does not delay the others; failing switches back off (5s → 15s → 30s).
- SNMP trap receiver for linkUp/Down, LLDP, PoE, fan/PSU/temperature, STP, IP conflict and CPU traps. Unknown traps are stored with their raw OID.
- Switch health: CPU (5s/60s/300s), per-process CPU, memory, Mbuf, temperature, fans, PSUs, reboot detection (sysUpTime going backwards).
- Ports and connected devices: LLDP, DHCP snooping, ARP, FDB with an OUI vendor dictionary, manual labels, and a connection timeline.
- AV checks: IGMP querier health, multicast group × port matrix, EEE/LPI, PAUSE frames (NDI4 is treated as an exception), CoS queue drops, DSCP trust, FrameTooLongs (MTU mismatch), UDLD, SFP DOM, PoE faults, STP/MSTP, double DHCP servers and rogue DHCP servers.
- Cross-switch audit against the NETGEAR ProAV guide: firmware mismatch, STP priority, querier placement, snooping consistency, uplink MTU and daisy-chain depth.
- Rehearsal baselines and diffs, incident markers (key `M`), `.avsight` session export, offline time-machine replay, and CSV/JSON export.
- Built-in Telnet/SSH terminal (xterm.js) with one-click `show` snippets. Connections are allowed only to registered switches, credentials are not stored, and sessions close after 10 minutes of inactivity.
- Built-in virtual M4250 lab (a real SNMPv2c agent plus fault injection and a mock CLI) for testing without hardware.

## Quick start
```powershell
# Try it without hardware: 4 virtual M4250 switches
.\avline-sight.exe --lab 4 --open
```
```powershell
# Production (WebUI on localhost:8080, traps on UDP 162)
.\avline-sight.exe --open
```
If UDP 162 cannot be bound, the app keeps running and shows a warning. This happens when the Windows "SNMP Trap" service holds the port or the app lacks permission. Change the trap port (for example `0.0.0.0:1162`) from the UI or with `--trap`.

| Flag | Default | Meaning |
|---|---|---|
| `--listen` | `127.0.0.1:8080` | WebUI address. Use `0.0.0.0:8080` to share on the LAN, together with the read-only lock. If the port is busy, the next free port is used. |
| `--trap` | `0.0.0.0:162` | SNMP trap listen address |
| `--data` | `<exe dir>/data` | Database and settings directory |
| `--lab N` | `0` | Start N virtual switches (1–20) and monitor them. The lab uses a separate database. |
| `--fast/--medium/--slow` | `3s/12s/45s` | Polling intervals |

## Checking SNMP trap reception

Traps let you notice failures immediately and reduce polling load. Monitoring continues without them, but short link drops are easier to miss.

**1. Check the app**: if the trap receiver status at the top says "listening", reception is ready. If it says it cannot listen, go to step 2. If a switch's "last trap" stays empty, check the switch's trap destination settings.

**2. Find what is using port 162** (PowerShell)
```powershell
Get-NetUDPEndpoint -LocalPort 162 -ErrorAction SilentlyContinue | Select-Object LocalAddress, OwningProcess
Get-Service SNMPTRAP -ErrorAction SilentlyContinue
```
- If nothing is listed, port 162 is free. Where administrator rights are required, run the app as administrator.
- If `SNMPTRAP` (the Windows "SNMP Trap" service) is Running, it holds port 162.

**3. If the port cannot be used** (pick one)
- Stop the Windows "SNMP Trap" service (admin PowerShell): `Stop-Service SNMPTRAP; Set-Service SNMPTRAP -StartupType Disabled`
- Listen on another port: start with `--trap 0.0.0.0:1162` and set the switch's trap destination port to 1162.
- Allow it through Windows Firewall (admin): `New-NetFirewallRule -DisplayName "AVLine-Sight Trap" -Direction Inbound -Protocol UDP -LocalPort 162 -Action Allow` (use your port if changed)

**4. Switch-side settings**: see the example in `docs/api.md` (not yet verified on real hardware).

## Watching from several PCs

Run the monitoring app (exe) on one PC only; other PCs open `http://<that PC's IP>:8080` in a browser (start with `--listen 0.0.0.0:8080` and use the read-only lock). Acknowledgements, operation phase, work mode and lock are reflected on all PCs immediately. Running the app on several PCs doubles the polling load on the switches and does not share acknowledgement state, so it is not recommended.

## Build
Requirements: Go 1.26+, Node.js 20+.
```powershell
pwsh scripts/build.ps1 -Version 1.0.0
```
This builds the web UI (`web/dist`), runs the Go tests, and produces `bin/avline-sight.exe`.

## Documentation
- API contract: [docs/api.md](docs/api.md)
- MIB basis: NETGEAR M4250 firmware 13.0.7.6 MIBs. Every OID in `internal/mib` was machine-checked against the MIB files.

## Known limitations
- Tested only against the built-in simulator; no real M4250 has been tested yet. On first connection to real hardware, run "Probe" and export a diagnostic dump to check vendor-specific differences, such as the SFP DOM scale (`DBmHundreths`, currently ÷100).
- The OUI dictionary covers only major vendors. QSC, Biamp, MA Lighting, Clear-Com and BirdDog are not included yet.
- SSH host keys are not pinned. The fingerprint is shown on connect for manual checking.
- SNMPv3, external notifications, sFlow and SNMP SET actions are planned for v2.
