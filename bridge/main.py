"""
main.py – FuseFlash Python bridge FastAPI application.

Exposes:
  POST /deploy              – deploy a new FuseFlashToken contract
  GET  /balance/{address}   – query FLSH balance
  POST /transfer            – ERC-20 transfer
  POST /tap                 – Tap-and-Go transfer (with validator signature)
  POST /tap/sign            – produce a tap-validator signature
  POST /pos                 – POS Checkout payment
  POST /claim               – Merkle whitelist claim
  GET  /logs/recent         – recent logged transactions
  GET  /logs/stats          – aggregate stats
  GET  /logs/events/{name}  – events by type
  GET  /logs/address/{addr} – events by address
"""

from __future__ import annotations

import os
from typing import Any

from dotenv import load_dotenv
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

load_dotenv()

from llm_logger import TransactionLogger  # noqa: E402  (after dotenv)
from pyethers import (  # noqa: E402
    DeployReceipt,
    FuseFlashClient,
    FuseFlashDeployer,
    TxResult,
    sign_tap_intent,
)

# ---------------------------------------------------------------------------
# App
# ---------------------------------------------------------------------------

app = FastAPI(
    title="FuseFlash Bridge",
    description="Python bridge for deploying and interacting with the FuseFlashToken contract, "
                "with LLM-powered transaction logging.",
    version="1.0.0",
)

_tx_logger = TransactionLogger()

# ---------------------------------------------------------------------------
# Request / response models
# ---------------------------------------------------------------------------


class DeployRequest(BaseModel):
    rpc_url: str = Field(..., description="JSON-RPC endpoint")
    private_key: str = Field(..., description="Deployer private key (hex)")
    tap_validator: str = Field(..., description="Tap-validator address")
    merkle_root: str = Field("0x" + "00" * 32, description="Initial Merkle root (hex, 32 bytes)")


class TransferRequest(BaseModel):
    rpc_url: str
    contract_address: str
    private_key: str
    to: str
    amount_flsh: float


class TapSignRequest(BaseModel):
    private_key: str = Field(..., description="Tap-validator private key")
    from_addr: str
    to_addr: str
    amount_flsh: float
    nonce: str = Field(..., description="32-byte nonce (hex)")


class TapRequest(BaseModel):
    rpc_url: str
    contract_address: str
    private_key: str
    from_addr: str
    to_addr: str
    amount_flsh: float
    nonce: str
    signature: str = Field(..., description="Tap-validator ECDSA signature (hex)")


class PosRequest(BaseModel):
    rpc_url: str
    contract_address: str
    private_key: str
    merchant: str
    amount_flsh: float
    session_id: str = Field(..., description="32-byte session identifier (hex)")


class ClaimRequest(BaseModel):
    rpc_url: str
    contract_address: str
    private_key: str
    amount_flsh: float
    proof: list[str] = Field(..., description="Merkle proof nodes as hex strings")


class BalanceResponse(BaseModel):
    address: str
    balance_flsh: float


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _hex_to_bytes(hex_str: str) -> bytes:
    clean = hex_str[2:] if hex_str.startswith("0x") else hex_str
    return bytes.fromhex(clean)


def _client(rpc_url: str, contract_address: str, private_key: str) -> FuseFlashClient:
    try:
        return FuseFlashClient(
            rpc_url=rpc_url,
            contract_address=contract_address,
            private_key=private_key,
        )
    except ConnectionError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc


# ---------------------------------------------------------------------------
# Routes
# ---------------------------------------------------------------------------


@app.post("/deploy", response_model=dict[str, Any], tags=["Contract"])
def deploy(req: DeployRequest) -> dict[str, Any]:
    """Deploy a new FuseFlashToken contract."""
    try:
        deployer = FuseFlashDeployer(rpc_url=req.rpc_url, private_key=req.private_key)
        receipt: DeployReceipt = deployer.deploy(
            tap_validator=req.tap_validator,
            merkle_root=_hex_to_bytes(req.merkle_root),
        )
        return {
            "contract_address": receipt.contract_address,
            "tx_hash": receipt.tx_hash,
            "block_number": receipt.block_number,
            "gas_used": receipt.gas_used,
        }
    except RuntimeError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except ConnectionError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc


@app.get("/balance/{address}", response_model=BalanceResponse, tags=["Token"])
def get_balance(
    address: str,
    rpc_url: str = os.getenv("RPC_URL", "http://127.0.0.1:8545"),
    contract_address: str = os.getenv("CONTRACT_ADDRESS", ""),
    private_key: str = os.getenv("WALLET_KEY", ""),
) -> BalanceResponse:
    """Query the FLSH balance of an address."""
    if not contract_address:
        raise HTTPException(status_code=400, detail="CONTRACT_ADDRESS env-var not set")
    cl = _client(rpc_url, contract_address, private_key)
    return BalanceResponse(address=address, balance_flsh=cl.balance_of(address))


@app.post("/transfer", response_model=dict[str, Any], tags=["Token"])
def transfer(req: TransferRequest) -> dict[str, Any]:
    """Transfer FLSH tokens."""
    cl = _client(req.rpc_url, req.contract_address, req.private_key)
    try:
        result: TxResult = cl.transfer(to=req.to, amount_flsh=req.amount_flsh)
    except Exception as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    _log_synthetic(
        "Transfer",
        tx_hash=result.tx_hash,
        block_number=result.block_number,
        args={"from": cl.wallet_address, "to": req.to, "value": int(req.amount_flsh * 10**18)},
    )
    return _tx_result(result)


@app.post("/tap/sign", response_model=dict[str, str], tags=["Tap-and-Go"])
def tap_sign(req: TapSignRequest) -> dict[str, str]:
    """
    Produce the ECDSA signature required by tapAndGo.

    The tap-validator service calls this endpoint with its private key to
    authorise a Tap-and-Go payment before broadcasting it on-chain.
    """
    nonce_bytes = _hex_to_bytes(req.nonce)
    sig = sign_tap_intent(
        private_key=req.private_key,
        from_addr=req.from_addr,
        to_addr=req.to_addr,
        amount_flsh=req.amount_flsh,
        nonce=nonce_bytes,
    )
    return {"signature": "0x" + sig.hex()}


@app.post("/tap", response_model=dict[str, Any], tags=["Tap-and-Go"])
def tap_and_go(req: TapRequest) -> dict[str, Any]:
    """Execute a Tap-and-Go P2P transfer."""
    cl = _client(req.rpc_url, req.contract_address, req.private_key)
    try:
        result = cl.tap_and_go(
            from_addr=req.from_addr,
            to_addr=req.to_addr,
            amount_flsh=req.amount_flsh,
            nonce=_hex_to_bytes(req.nonce),
            signature=_hex_to_bytes(req.signature),
        )
    except Exception as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    _log_synthetic(
        "TapAndGo",
        tx_hash=result.tx_hash,
        block_number=result.block_number,
        args={
            "from": req.from_addr,
            "to": req.to_addr,
            "amount": int(req.amount_flsh * 10**18),
            "nonce": _hex_to_bytes(req.nonce),
        },
    )
    return _tx_result(result)


@app.post("/pos", response_model=dict[str, Any], tags=["POS Checkout"])
def pos_checkout(req: PosRequest) -> dict[str, Any]:
    """Execute a QR-based POS payment."""
    cl = _client(req.rpc_url, req.contract_address, req.private_key)
    try:
        result = cl.pos_checkout(
            merchant=req.merchant,
            amount_flsh=req.amount_flsh,
            session_id=_hex_to_bytes(req.session_id),
        )
    except Exception as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    _log_synthetic(
        "PosCheckout",
        tx_hash=result.tx_hash,
        block_number=result.block_number,
        args={
            "payer": cl.wallet_address,
            "merchant": req.merchant,
            "amount": int(req.amount_flsh * 10**18),
            "sessionId": _hex_to_bytes(req.session_id),
        },
    )
    return _tx_result(result)


@app.post("/claim", response_model=dict[str, Any], tags=["Whitelist"])
def claim_whitelist(req: ClaimRequest) -> dict[str, Any]:
    """Claim a Merkle-whitelist allocation."""
    cl = _client(req.rpc_url, req.contract_address, req.private_key)
    proof_bytes = [_hex_to_bytes(p) for p in req.proof]
    try:
        result = cl.claim_whitelist(amount_flsh=req.amount_flsh, proof=proof_bytes)
    except Exception as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    _log_synthetic(
        "WhitelistClaimed",
        tx_hash=result.tx_hash,
        block_number=result.block_number,
        args={"claimant": cl.wallet_address, "amount": int(req.amount_flsh * 10**18)},
    )
    return _tx_result(result)


# ── Log query endpoints ──────────────────────────────────────────────────────


@app.get("/logs/recent", tags=["Logs"])
def recent_logs(n: int = 20) -> list[dict[str, Any]]:
    """Return the *n* most-recently logged transactions."""
    return _tx_logger.recent(n=n)


@app.get("/logs/stats", tags=["Logs"])
def log_stats() -> dict[str, Any]:
    """Return aggregate statistics across all persisted log entries."""
    return _tx_logger.stats()


@app.get("/logs/events/{event_name}", tags=["Logs"])
def logs_by_event(event_name: str) -> list[dict[str, Any]]:
    """Return all logged entries for a specific event type."""
    return _tx_logger.by_event(event_name)


@app.get("/logs/address/{address}", tags=["Logs"])
def logs_by_address(address: str) -> list[dict[str, Any]]:
    """Return all logged entries involving *address*."""
    return _tx_logger.by_address(address)


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _tx_result(r: TxResult) -> dict[str, Any]:
    return {
        "tx_hash": r.tx_hash,
        "block_number": r.block_number,
        "gas_used": r.gas_used,
        "status": r.status,
    }


def _log_synthetic(
    event_name: str,
    tx_hash: str,
    block_number: int,
    args: dict[str, Any],
) -> None:
    """Log a synthetic event dict (constructed from the API request/response)."""
    try:
        _tx_logger.log_event(
            {
                "event": event_name,
                "transactionHash": bytes.fromhex(tx_hash[2:] if tx_hash.startswith("0x") else tx_hash),
                "blockNumber": block_number,
                "args": args,
            }
        )
    except Exception as exc:  # noqa: BLE001
        # Logging failures must not propagate back to the API caller.
        import logging as _logging
        _logging.getLogger(__name__).warning("Failed to log event: %s", exc)
