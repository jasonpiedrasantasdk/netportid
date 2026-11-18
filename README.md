# netportid

`netportid` is a Linux terminal UI for monitoring local network ports and identifying which processes own them. It captures live traffic, aggregates activity by local port/protocol, detects common application protocols, and maps ports to processes via `/proc`.

> Companion project: [netview](https://github.com/jasonpiedrasantasdk/netview) — a connection-centric live traffic inspector with detail views and pcap export.

## Features

- Live packet capture on one or more interfaces (or all active, non-loopback interfaces by default)
- Port-centric view with protocol, recent activity direction, TX/RX bytes, and owning process
- Protocol detection heuristics for TCP/UDP traffic, including TLS (SNI, version, ALPN), HTTP, DNS, QUIC, SSH, and DHCP
- Process attribution by correlating `/proc/net/tcp{,6}`, `/proc/net/udp{,6}`, and `/proc/*/fd` socket inodes
- Toggle between active traffic and all/listening ports, port search, pause, and refresh
- Filters common local noise by default (loopback, link-local/multicast, mDNS, LLMNR, SSDP)

## Screenshot

<!-- TODO: add screenshot/demo -->
_Screenshot coming soon._

## Requirements

- **Linux** (uses `AF_PACKET` capture and `/proc` for process mapping)
- **Root privileges** (or equivalent raw-capture capabilities) to capture packets
- **Go 1.22+** to build from source (the module currently declares `go 1.22.0`)

## Install / Build

### Build from source

```bash
git clone https://github.com/jasonpiedrasantasdk/netportid.git
cd netportid
go build -o netportid .
```

Run:

```bash
sudo ./netportid [flags]
```

### Install with Go

Once a tagged release is available:

```bash
go install github.com/jasonpiedrasantasdk/netportid@latest
sudo "$(go env GOPATH)/bin/netportid"
```

## Usage

```bash
sudo netportid [-i iface1,iface2]
```

**Flags**

- `-i` — Comma-separated interfaces to monitor (default: all active, non-loopback interfaces)

## Controls

- `q` / `Ctrl+C` — Quit
- `a` — Toggle active ports / all ports (seeds listening ports from the process map)
- `p` — Pause/resume capture updates
- `s` — Search by port number (`Enter` to search, `Esc` to cancel)
- `r` — Refresh process map
- `↑`/`↓` or `j`/`k` — Move selection
- `g` / `b` — Jump to top/bottom

**Columns:** Port, Proto, Dir/activity, TX, RX, Process. Entries are sorted by most recently seen activity.

## How it works & limitations

- Traffic is observed from captured packets and aggregated per local port/protocol; process ownership is inferred from system socket state and may be unavailable or delayed in some cases.
- Protocol identification is heuristic (based on ports/payload inspection) and may be incomplete or imprecise.
- Requires sufficient privileges to read packets and relevant `/proc` entries.
- Linux-only.

## Security & privacy

`netportid` can reveal active ports, communication patterns, observable protocol metadata (e.g. SNI/DNS/HTTP details), and local process information. Use only on systems and networks you are authorized to monitor.

## Author

- [Jason Piedrasanta](https://software.piedrasanta.space)

## License

[MIT](LICENSE) © 2026 Jason Piedrasanta
