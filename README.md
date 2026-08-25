# qo-learn-tool

A self-hosted platform for running Linux challenges in isolated sandboxes — built for OSC workshops and events. Students join from a browser, get a root shell in a locked-down sandbox, solve levels, and appear on a live leaderboard.

```
Browser (xterm.js) ──WebSocket──> Go server ──PTY──> qo sandbox (namespaces + cgroups)
                                       │
                                       └─ challenge validation, scores, admin
```

## Components

| Piece | What it does |
|-------|-------------|
| [`qo/`](qo/) | CLI that builds encrypted challenge archives and runs the sandbox (`qo build`, `qo start`, `qo meta`) |
| `deploy/qo-init.c` | C helper spawned inside the sandbox: namespaces, pivot_root, cgroups, devices |
| `server/` | HTTP/WebSocket server: sessions, terminal, leaderboard, admin API |
| `frontend/` | Terminal UI (vendored xterm.js), login, admin, leaderboard pages |
| `mockup-challenges/` | Example challenge packs, including `quantum-realm` |

## Requirements

- Linux with user namespaces enabled (any recent distro)
- Go 1.24+, `gcc` with `libseccomp` dev headers
- Root (sandboxes need it)

Arch: `sudo pacman -S go gcc libseccomp`

## Build

```bash
./deploy/run.sh
```

This produces `deploy/qo`, `deploy/qo-init`, and `deploy/server` (frontend is embedded — only the binaries are needed on the event machine).

## Create a challenge

A challenge pack is a folder with a `meta.yaml` and one directory per level:

```
my-challenge/
├── meta.yaml            # title, story, questions, hints
├── level1/
│   ├── check.sh         # executable; exit 0 = level passed
│   └── ...              # files the student works with
└── level2/
    └── check.sh
```

See `mockup-challenges/quantum-realm/` for a working example.

Encrypt it (unlock time prevents early access):

```bash
./deploy/qo build -f my-challenge -p <password> -k <startkey> \
  -u "2026-08-30 09:00" -o my-challenge.enc
```

## Run standalone (one student, terminal)

```bash
sudo ./deploy/qo start -i <student-id> -a my-challenge.enc \
  -p <password> -k <startkey> -d 60m
```

## Run the server (many students, browser)

```bash
sudo -E EVENT_CODE=osc26 ADMIN_SECRET=<admin-token> \
  QO_BINARY_PATH=$PWD/deploy/qo ARCHIVE_PATH=$PWD/my-challenge.enc \
  ARCHIVE_PASSWORD=<password> ARCHIVE_KEY=<startkey> QO_DURATION=60m \
  PORT=8080 ./deploy/server
```

Students open `http://<host>:8080`, enter the event code and their name, and get a terminal. Useful URLs:

| URL | Purpose |
|-----|---------|
| `/leaderboard` | Live scoreboard |
| `/admin` | Session management (kill/shutdown) — needs `ADMIN_SECRET` |

### Server environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `EVENT_CODE` | — | Code students must enter to join |
| `ADMIN_SECRET` | — | Admin API token |
| `ARCHIVE_PATH` / `ARCHIVE_PASSWORD` / `ARCHIVE_KEY` | — | Challenge archive and credentials |
| `QO_BINARY_PATH` | — | Path to the `qo` binary |
| `QO_DURATION` | — | Session length per student (e.g. `60m`) |
| `MAX_CONCURRENT` | `8` | Max simultaneous sandboxes |
| `IDLE_TIMEOUT` | `600` | Seconds of inactivity before a session is closed (warns 60s prior) |
| `GRACE_PERIOD` | `45` | Seconds a disconnected session stays recoverable |
| `PORT` | `8080` | Listen port |

Remote access: point [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) at the port.

## Inside the sandbox

Students are root in an isolated environment (user/mount/PID/net/IPC/cgroup namespaces, dropped capabilities, seccomp optional via `QO_SECCOMP=enforce`). Custom commands:

| Command | Action |
|---------|--------|
| `quest [n]` | Show question for current level n |
| `go [answer]` | Validate the current level (some levels take an answer) |
| `level <n>` | Switch level |
| `hint` | Hint for current level |
| `map` / `status` | Progress overview |

## Tests

```bash
cd server && go test ./...
```

## Team & Thanks

qo was born when **Ahmed Yasser** pitched it as a sandbox idea for Summer CTF and built the first product version. Huge thanks to everyone who brought it to life:

| Name | Role | GitHub | LinkedIn |
|------|------|--------|----------|
| [Ahmed Yasser](https://github.com/ahmedYasserM) | Started qo as a sandbox idea for Summer CTF — first product version | [@ahmedYasserM](https://github.com/ahmedYasserM) | [ahmedyasser2592](https://www.linkedin.com/in/ahmedyasser2592) |
| [Amna](https://github.com/thisisamna) | Contributor | [@thisisamna](https://github.com/thisisamna) | — |
| [Zyad Salah](https://github.com/zyad-elkhewekh) | Contributor | [@zyad-elkhewekh](https://github.com/zyad-elkhewekh) | — |
| [Mohammed Naser](https://github.com/MohammedNaser28) | Contributor | [@MohammedNaser28](https://github.com/MohammedNaser28) | [mohammed-naser](https://www.linkedin.com/in/mohammed-naser-2253a0235/) |

Thank you all for making qo possible. 💙
