"""
fuseflash_bridge.py — FastAPI bridge service connecting the FuseFlash Go node
to the FuseSpark testnet (EVM, Chain ID 123) via Web3.py and Blockscout.

Endpoints:
  GET  /health                         — liveness probe
  GET  /network                        — FuseSpark network info
  GET  /address/{addr}                 — address summary (balance + tx count)
  GET  /transactions/{addr}            — paginated tx list from Blockscout
  POST /broadcast                      — sign + broadcast a raw flash-tx
  GET  /block/latest                   — latest block number
  GET  /stats                          — Blockscout network stats

Run:
  uvicorn fuseflash_bridge:app --host 0.0.0.0 --port 8000 --reload

Environment variables:
  FUSEFLASH_RPC_URL          (default: https://rpc.fusespark.io)
  FUSEFLASH_PRIVATE_KEY      hex private key for the bridge hot wallet (optional)
"""

import os
from contextlib import asynccontextmanager
from typing import Any, Dict, Optional

import httpx
from fastapi import FastAPI, HTTPException, Query
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, field_validator
from web3 import Web3

from blockscout import BlockscoutClient

# ── Config ───────────────────────────────────────────────────────────────────

RPC_URL = os.getenv("FUSEFLASH_RPC_URL", "https://rpc.fusespark.io")
PRIVATE_KEY = os.getenv("FUSEFLASH_PRIVATE_KEY", "")
CHAIN_ID = 123

# ── App lifecycle ─────────────────────────────────────────────────────────────

_blockscout: Optional[BlockscoutClient] = None
_w3: Optional[Web3] = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _blockscout, _w3
    _w3 = Web3(Web3.HTTPProvider(RPC_URL))
    _blockscout = BlockscoutClient()
    yield
    if _blockscout:
        await _blockscout.close()


app = FastAPI(
    title="FuseFlash Bridge",
    description="Go ↔ Python ↔ FuseSpark testnet bridge for the FuseFlash P2P wallet.",
    version="0.1.0",
    lifespan=lifespan,
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

# ── Models ────────────────────────────────────────────────────────────────────


class BroadcastRequest(BaseModel):
    from_address: str
    to_address: str
    amount_wei: str  # wei as string to avoid overflow
    nonce: Optional[int] = None
    gas: int = 21_000
    gas_price_gwei: float = 1.0

    @field_validator("from_address", "to_address")
    @classmethod
    def must_be_evm_address(cls, v: str) -> str:
        if not Web3.is_address(v):
            raise ValueError(f"Invalid EVM address: {v}")
        return Web3.to_checksum_address(v)


class BroadcastResponse(BaseModel):
    tx_hash: str
    explorer_url: str


# ── Helpers ───────────────────────────────────────────────────────────────────


def _w3_or_raise() -> Web3:
    if _w3 is None:
        raise HTTPException(503, "Web3 provider not initialised")
    return _w3


def _bs_or_raise() -> BlockscoutClient:
    if _blockscout is None:
        raise HTTPException(503, "Blockscout client not initialised")
    return _blockscout


# ── Routes ────────────────────────────────────────────────────────────────────


@app.get("/health", tags=["system"])
async def health() -> Dict[str, Any]:
    w3 = _w3_or_raise()
    connected = w3.is_connected()
    return {
        "status": "ok" if connected else "degraded",
        "rpc_connected": connected,
        "chain_id": CHAIN_ID,
    }


@app.get("/network", tags=["network"])
async def network_info() -> Dict[str, Any]:
    return {
        "name": "FuseSpark Testnet",
        "chain_id": CHAIN_ID,
        "currency": "SPARK",
        "rpc_url": RPC_URL,
        "explorer_url": "https://explorer.fusespark.io",
        "faucet_url": "https://faucet.fusespark.io",
    }


@app.get("/address/{addr}", tags=["address"])
async def get_address(addr: str) -> Dict[str, Any]:
    if not Web3.is_address(addr):
        raise HTTPException(400, f"Invalid address: {addr}")
    addr = Web3.to_checksum_address(addr)
    w3 = _w3_or_raise()
    bs = _bs_or_raise()
    try:
        balance_wei = w3.eth.get_balance(addr)
        bs_data = await bs.get_address(addr)
    except httpx.HTTPError as exc:
        raise HTTPException(502, f"Blockscout error: {exc}") from exc
    return {
        "address": addr,
        "balance_wei": str(balance_wei),
        "balance_spark": str(Web3.from_wei(balance_wei, "ether")),
        "blockscout": bs_data,
        "explorer_url": f"https://explorer.fusespark.io/address/{addr}",
    }


@app.get("/transactions/{addr}", tags=["transactions"])
async def get_transactions(
    addr: str,
    page: int = Query(1, ge=1),
    limit: int = Query(20, ge=1, le=100),
) -> Dict[str, Any]:
    if not Web3.is_address(addr):
        raise HTTPException(400, f"Invalid address: {addr}")
    bs = _bs_or_raise()
    try:
        data = await bs.get_transactions(addr, page=page, limit=limit)
    except httpx.HTTPError as exc:
        raise HTTPException(502, f"Blockscout error: {exc}") from exc
    return data


@app.get("/block/latest", tags=["blocks"])
async def latest_block() -> Dict[str, Any]:
    w3 = _w3_or_raise()
    number = w3.eth.block_number
    return {"block_number": number}


@app.get("/stats", tags=["network"])
async def network_stats() -> Dict[str, Any]:
    bs = _bs_or_raise()
    try:
        return await bs.get_stats()
    except httpx.HTTPError as exc:
        raise HTTPException(502, f"Blockscout error: {exc}") from exc


@app.post("/broadcast", response_model=BroadcastResponse, tags=["transactions"])
async def broadcast_tx(req: BroadcastRequest) -> BroadcastResponse:
    """Sign and broadcast a FuseFlash transaction to the FuseSpark testnet."""
    if not PRIVATE_KEY:
        raise HTTPException(501, "Bridge private key not configured (FUSEFLASH_PRIVATE_KEY)")
    w3 = _w3_or_raise()
    account = w3.eth.account.from_key(PRIVATE_KEY)
    nonce = req.nonce if req.nonce is not None else w3.eth.get_transaction_count(account.address)
    tx = {
        "chainId": CHAIN_ID,
        "from": account.address,
        "to": req.to_address,
        "value": int(req.amount_wei),
        "gas": req.gas,
        "gasPrice": Web3.to_wei(req.gas_price_gwei, "gwei"),
        "nonce": nonce,
    }
    signed = account.sign_transaction(tx)
    tx_hash = w3.eth.send_raw_transaction(signed.raw_transaction)
    hex_hash = tx_hash.hex()
    return BroadcastResponse(
        tx_hash=hex_hash,
        explorer_url=f"https://explorer.fusespark.io/tx/{hex_hash}",
    )
