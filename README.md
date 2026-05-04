# ⚡ FuseFlash — P2P Smart Wallet on FuseSpark Testnet

[![FuseSpark CI](https://github.com/dlema861011010/dlema861011010/actions/workflows/fusespark-ci.yml/badge.svg)](https://github.com/dlema861011010/dlema861011010/actions/workflows/fusespark-ci.yml)

A full-stack Web3 development environment documenting the build of **FuseFlash** — a peer-to-peer flash-transaction network — bridging Go and Python on the [FuseSpark](https://fusespark.io) testnet (Chain ID 123).

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Ember.js Smart Wallet              │
│  wallet-connect · send-tx · tx-history · bridge UI  │
│         ethers.js  ·  @stripe/stripe-js              │
└────────────────────┬────────────────────────────────┘
                     │ HTTP (port 4200 → 8000)
┌────────────────────▼────────────────────────────────┐
│              Python FastAPI Bridge                   │
│     Web3.py · Blockscout v2 API · eth-account        │
└────────────────────┬────────────────────────────────┘
          ┌──────────┴──────────────┐
          │ JSON-RPC                │ Blockscout REST
┌─────────▼──────┐        ┌────────▼──────────────┐
│ FuseSpark RPC  │        │ explorer.fusespark.io  │
│  (EVM, ID 123) │        │  (Blockscout v2)       │
└────────────────┘        └───────────────────────┘
          ▲
          │ P2P TCP (port 7777)
┌─────────┴──────────────────────────────────────────┐
│            FuseFlash Go P2P Node                    │
│   base58 · wallet · p2p flash-tx protocol           │
└────────────────────────────────────────────────────┘
```

## Repository Layout

```
.
├── fuseflash/              # Go module — P2P node, wallet, base58
│   ├── base58/             # Base58Check encoding
│   ├── wallet/             # ECDSA key gen, signing, EVM address derivation
│   ├── p2p/                # TCP peer, FuseFlash message protocol
│   └── cmd/node/           # Runnable P2P node
├── smart-wallet/           # Ember.js SPA
│   └── app/
│       ├── services/       # web3.js · stripe.js · blockscout.js
│       ├── components/     # WalletConnect · SendTransaction · TxHistory
│       └── templates/      # Glimmer templates + app CSS
├── bridge/                 # Python FastAPI bridge
│   ├── fuseflash_bridge.py # REST API + Web3.py integration
│   └── blockscout.py       # Async Blockscout v2 client
├── docker/                 # Docker stack
│   ├── docker-compose.yml
│   ├── Dockerfile.fuseflash
│   ├── Dockerfile.bridge
│   └── Dockerfile.smartwallet
├── gcp/                    # GCP Cloud Run + Cloud Build configs
│   ├── cloudrun-bridge.yaml
│   └── cloudbuild.yaml
├── .devcontainer/          # VS Code Dev Container
│   └── devcontainer.json
├── scripts/
│   └── dev-setup.sh        # One-shot setup (all environments)
└── .github/workflows/
    └── fusespark-ci.yml    # CI: Go · Python · Node · Docker · GCP deploy
```

---

## Quick Start

### Docker Desktop (recommended — includes Gordon AI support)

```bash
docker compose -f docker/docker-compose.yml up --build
```

| Service | URL |
|---------|-----|
| Smart Wallet UI | http://localhost:4200 |
| Bridge API | http://localhost:8000 |
| Bridge docs | http://localhost:8000/docs |
| FuseFlash P2P node | tcp://localhost:7777 |

### VS Code Dev Container

1. Open this repo in VS Code.
2. When prompted, click **"Reopen in Container"** (or run `Dev Containers: Reopen in Container`).
3. All tools (Go 1.21, Node 20, Python 3.12, Docker, GitHub CLI, Google Cloud Code) are pre-installed.
4. Ports 7777, 8000, and 4200 are forwarded automatically.

### Termux (Android)

```bash
pkg install golang nodejs python git
bash scripts/dev-setup.sh

# Run individually:
cd fuseflash && go run ./cmd/node           # P2P node
cd bridge && uvicorn fuseflash_bridge:app --reload   # Python bridge
cd smart-wallet && npm start               # Ember wallet
```

> Docker is not available in standard Termux; run services individually as above.

### Bare Linux / GCP Cloud Shell

```bash
bash scripts/dev-setup.sh
```

---

## FuseSpark Testnet

| Property | Value |
|----------|-------|
| Chain ID | `123` |
| Currency | `SPARK` |
| RPC URL | `https://rpc.fusespark.io` |
| Explorer | https://explorer.fusespark.io (Blockscout) |
| Faucet | https://faucet.fusespark.io |

### Add to MetaMask / Web3 wallet

The smart wallet UI auto-prompts to add FuseSpark when you click **Connect Wallet**. Or add manually:

- **Network name**: FuseSpark Testnet
- **RPC URL**: `https://rpc.fusespark.io`
- **Chain ID**: `123`
- **Symbol**: `SPARK`
- **Block explorer**: `https://explorer.fusespark.io`

---

## Components

### `fuseflash/` — Go P2P Node

```bash
cd fuseflash
go test ./...                    # run all tests
go run ./cmd/node -listen :7777  # start node
go run ./cmd/node -listen :7778 -peers localhost:7777  # second node
```

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `FUSEFLASH_LISTEN` | `:7777` | TCP listen address |
| `FUSEFLASH_PEERS` | `` | Comma-separated bootstrap peers |
| `FUSEFLASH_PRIVATE_KEY` | *(generated)* | Hex-encoded private key |

### `bridge/` — Python FastAPI Bridge

```bash
cd bridge
pip install -r requirements.txt
uvicorn fuseflash_bridge:app --host 0.0.0.0 --port 8000 --reload
```

API docs: http://localhost:8000/docs

Key endpoints:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/network` | FuseSpark chain info |
| GET | `/address/{addr}` | Balance + tx count |
| GET | `/transactions/{addr}` | Paginated tx list |
| GET | `/block/latest` | Latest block number |
| GET | `/stats` | Blockscout network stats |
| POST | `/broadcast` | Sign + broadcast flash-tx |

### `smart-wallet/` — Ember.js UI

```bash
cd smart-wallet
npm install
npm start      # http://localhost:4200
npm test       # run test suite
npm run build  # production build -> dist/
```

Key services:
- **`web3`** — MetaMask connection, EVM tx, FuseSpark chain switching
- **`blockscout`** — Blockscout v2 REST API client
- **`stripe`** — Stripe card element + payment confirmation

---

## GCP Deployment

1. Edit `gcp/cloudbuild.yaml` and set `_PROJECT_ID` to your GCP project.
2. Add GitHub secrets: `GCP_SA_KEY` (service account JSON) and `GCP_PROJECT_ID`.
3. Push to `main` — CI automatically submits a Cloud Build and deploys the bridge to Cloud Run.

Manual deploy:

```bash
gcloud builds submit --config gcp/cloudbuild.yaml \
  --substitutions _PROJECT_ID=my-project,SHORT_SHA=$(git rev-parse --short HEAD) .
```

---

## Technology Stack

| Layer | Technology |
|-------|-----------|
| P2P Node | Go 1.21, standard library + golang.org/x/crypto |
| Bridge API | Python 3.12, FastAPI, Web3.py, Blockscout |
| Frontend | Ember.js 6, ethers.js v6, Stripe.js v5 |
| Blockchain | FuseSpark Testnet (EVM, Chain ID 123) |
| Explorer | Blockscout v2 |
| Container | Docker, Docker Compose (Gordon-ready) |
| Cloud | GCP Cloud Run + Cloud Build |
| IDE | VS Code Dev Containers |
| Mobile | Termux (Android) |
| CI/CD | GitHub Actions |

---

## Development Environments

This project is built simultaneously across:

- **VS Code** — Dev Container with Go, Python, Node, Google Cloud Code, GitHub Copilot
- **GCP** — Cloud Shell for one-off builds; Cloud Run for the bridge service
- **Docker Desktop** — Full local stack via docker compose; Gordon AI for image management
- **Termux** — Android-native development without Docker (services run individually)
- **GitHub Copilot** — AI-assisted development and documentation

---

## License

MIT
