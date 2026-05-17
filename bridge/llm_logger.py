"""
llm_logger.py
=============
LLM-powered transaction logger for the FuseFlash blockchain.

Architecture
------------
1. ``TransactionDecoder``  – decodes raw EVM log entries (or structured
   contract-event dicts from pyethers) into a normalised ``TxEvent`` record.
2. ``LLMSummariser``       – sends the normalised event to an OpenAI-compatible
   LLM and receives a human-readable narrative summary.
3. ``TransactionLogger``   – high-level entry point: decode → summarise →
   persist.  Persists to a JSONL log file and exposes query helpers.

The LLM model and API key are read from environment variables so no secrets
are hard-coded.  The module degrades gracefully when no API key is present –
summaries are generated rule-based instead.

Usage example
-------------
    from llm_logger import TransactionLogger, LLMSummariser

    llm = LLMSummariser()          # reads OPENAI_API_KEY from env
    tx_logger = TransactionLogger(log_path="logs/fuseflash.jsonl", summariser=llm)

    # Feed a raw contract-event dict (as returned by FuseFlashClient)
    tx_logger.log_event(event_dict)

    # Or log all events polled from the chain
    for event in client.get_tap_and_go_events(from_block=last_seen):
        tx_logger.log_event(event)

    # Query recent logs
    entries = tx_logger.recent(n=10)
"""

from __future__ import annotations

import json
import logging
import os
import re
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_KNOWN_EVENT_NAMES = {
    "TapAndGo",
    "PosCheckout",
    "WhitelistClaimed",
    "Transfer",
    "MerkleRootUpdated",
    "TapValidatorUpdated",
}

_DECIMALS = 18
_ONE_FLSH = 10**_DECIMALS

_DEFAULT_MODEL = os.getenv("FUSEFLASH_LLM_MODEL", "gpt-4o-mini")
_DEFAULT_LOG_PATH = os.getenv("FUSEFLASH_LOG_PATH", "logs/fuseflash_txns.jsonl")

# ---------------------------------------------------------------------------
# Data model
# ---------------------------------------------------------------------------


@dataclass
class TxEvent:
    """Normalised representation of a single on-chain FuseFlash event."""

    event_name: str
    tx_hash: str
    block_number: int
    timestamp: str  # ISO-8601 UTC
    # Parsed human-readable fields
    from_addr: str = ""
    to_addr: str = ""
    amount_flsh: float = 0.0
    nonce: str = ""
    session_id: str = ""
    claimant: str = ""
    new_root: str = ""
    new_validator: str = ""
    # Raw decoded args (full fidelity)
    raw_args: dict[str, Any] = field(default_factory=dict)
    # LLM narrative
    summary: str = ""


# ---------------------------------------------------------------------------
# TransactionDecoder
# ---------------------------------------------------------------------------


class TransactionDecoder:
    """
    Convert raw web3 contract-event dicts (as returned by pyethers helpers or
    web3.py ``get_all_entries()``) into normalised ``TxEvent`` objects.
    """

    def decode(self, event: dict[str, Any]) -> TxEvent:
        """
        Decode a single contract-event dict.

        Handles both the ``AttributeDict`` format returned by web3.py's
        ``event_filter.get_all_entries()`` and the plain-dict format returned
        by pyethers helpers.
        """
        event_name = event.get("event", "Unknown")
        args = dict(event.get("args", {}))
        tx_hash = _hex_or_str(event.get("transactionHash", b""))
        block_number = int(event.get("blockNumber", 0))
        timestamp = datetime.now(tz=timezone.utc).isoformat()

        te = TxEvent(
            event_name=event_name,
            tx_hash=tx_hash,
            block_number=block_number,
            timestamp=timestamp,
            raw_args=args,
        )

        if event_name == "TapAndGo":
            te.from_addr = args.get("from", "")
            te.to_addr = args.get("to", "")
            te.amount_flsh = _wei_to_flsh(args.get("amount", 0))
            te.nonce = _hex_or_str(args.get("nonce", b""))

        elif event_name == "PosCheckout":
            te.from_addr = args.get("payer", "")
            te.to_addr = args.get("merchant", "")
            te.amount_flsh = _wei_to_flsh(args.get("amount", 0))
            te.session_id = _hex_or_str(args.get("sessionId", b""))

        elif event_name == "WhitelistClaimed":
            te.claimant = args.get("claimant", "")
            te.amount_flsh = _wei_to_flsh(args.get("amount", 0))

        elif event_name == "Transfer":
            te.from_addr = args.get("from", "")
            te.to_addr = args.get("to", "")
            te.amount_flsh = _wei_to_flsh(args.get("value", 0))

        elif event_name == "MerkleRootUpdated":
            te.new_root = _hex_or_str(args.get("newRoot", b""))

        elif event_name == "TapValidatorUpdated":
            te.new_validator = args.get("newValidator", "")

        return te


# ---------------------------------------------------------------------------
# LLMSummariser
# ---------------------------------------------------------------------------


class LLMSummariser:
    """
    Generate human-readable narrative summaries of FuseFlash transactions
    using an OpenAI-compatible LLM.

    If ``OPENAI_API_KEY`` is not set, the summariser falls back to a
    deterministic rule-based summary so the logger works without any API key.

    Parameters
    ----------
    api_key:
        OpenAI API key.  Defaults to the ``OPENAI_API_KEY`` environment
        variable.
    model:
        Model name.  Defaults to ``FUSEFLASH_LLM_MODEL`` env-var or
        ``gpt-4o-mini``.
    base_url:
        Optional custom OpenAI-compatible base URL (e.g. for local Ollama or
        Azure endpoints).  Defaults to the ``OPENAI_BASE_URL`` env-var.
    """

    _SYSTEM_PROMPT = (
        "You are the FuseFlash transaction analyst.  You receive structured "
        "JSON describing a single on-chain event from the FuseFlashToken "
        "smart contract and respond with a concise, human-readable one or two "
        "sentence narrative summary suitable for a transaction feed.  Include "
        "the relevant addresses (abbreviated to first-6/last-4 chars), token "
        "amounts formatted to 4 decimal places, and the event type.  Do not "
        "include raw JSON in your response."
    )

    def __init__(
        self,
        api_key: str | None = None,
        model: str = _DEFAULT_MODEL,
        base_url: str | None = None,
    ) -> None:
        self._model = model
        self._client: Any = None  # lazy init

        resolved_key = api_key or os.getenv("OPENAI_API_KEY")
        if resolved_key:
            try:
                from openai import OpenAI  # type: ignore[import-untyped]

                kwargs: dict[str, Any] = {"api_key": resolved_key}
                resolved_url = base_url or os.getenv("OPENAI_BASE_URL")
                if resolved_url:
                    kwargs["base_url"] = resolved_url
                self._client = OpenAI(**kwargs)
                logger.info("LLMSummariser: using model %s", model)
            except ImportError:
                logger.warning("openai package not installed; using rule-based summaries")
        else:
            logger.info("LLMSummariser: no API key – using rule-based summaries")

    def summarise(self, event: TxEvent) -> str:
        """Return a human-readable summary for *event*."""
        if self._client is not None:
            return self._llm_summary(event)
        return self._rule_based_summary(event)

    def _llm_summary(self, event: TxEvent) -> str:
        try:
            payload = {
                "event": event.event_name,
                "block": event.block_number,
                "tx": event.tx_hash[:12] + "…",
                "from": _abbrev(event.from_addr),
                "to": _abbrev(event.to_addr),
                "amount_flsh": round(event.amount_flsh, 4),
                "nonce": event.nonce[:10] + "…" if event.nonce else None,
                "session_id": event.session_id[:10] + "…" if event.session_id else None,
                "claimant": _abbrev(event.claimant),
            }
            # Remove None/empty fields to reduce token count
            payload = {k: v for k, v in payload.items() if v}

            response = self._client.chat.completions.create(
                model=self._model,
                messages=[
                    {"role": "system", "content": self._SYSTEM_PROMPT},
                    {"role": "user", "content": json.dumps(payload)},
                ],
                max_tokens=120,
                temperature=0.3,
            )
            return response.choices[0].message.content.strip()
        except Exception as exc:  # noqa: BLE001
            logger.warning("LLM call failed (%s); falling back to rule-based summary", exc)
            return self._rule_based_summary(event)

    @staticmethod
    def _rule_based_summary(event: TxEvent) -> str:
        """Deterministic fallback summary when no LLM is available."""
        name = event.event_name
        amt = f"{event.amount_flsh:.4f} FLSH"

        if name == "TapAndGo":
            return (
                f"⚡ Tap-and-Go: {_abbrev(event.from_addr)} paid "
                f"{_abbrev(event.to_addr)} {amt} "
                f"(nonce {event.nonce[:8]}…) in block {event.block_number}."
            )
        if name == "PosCheckout":
            return (
                f"🛒 POS Checkout: {_abbrev(event.from_addr)} paid merchant "
                f"{_abbrev(event.to_addr)} {amt} "
                f"(session {event.session_id[:8]}…) in block {event.block_number}."
            )
        if name == "WhitelistClaimed":
            return (
                f"✅ Whitelist Claim: {_abbrev(event.claimant)} claimed "
                f"{amt} in block {event.block_number}."
            )
        if name == "Transfer":
            from_label = "Mint" if event.from_addr == "0x" + "0" * 40 else _abbrev(event.from_addr)
            return (
                f"💸 Transfer: {from_label} → {_abbrev(event.to_addr)} "
                f"{amt} in block {event.block_number}."
            )
        if name == "MerkleRootUpdated":
            return (
                f"🌳 Merkle Root updated to {event.new_root[:12]}… "
                f"in block {event.block_number}."
            )
        if name == "TapValidatorUpdated":
            return (
                f"🔑 Tap Validator changed to {_abbrev(event.new_validator)} "
                f"in block {event.block_number}."
            )
        return f"📋 {name} event in block {event.block_number} (tx {event.tx_hash[:12]}…)."


# ---------------------------------------------------------------------------
# TransactionLogger
# ---------------------------------------------------------------------------


class TransactionLogger:
    """
    High-level logger: decode → summarise → persist → query.

    Logs are persisted as newline-delimited JSON (JSONL) so they can be
    streamed, rotated, and parsed by any standard log tooling.

    Parameters
    ----------
    log_path:
        Path to the JSONL log file.  Created (including parent dirs) if absent.
    summariser:
        ``LLMSummariser`` instance.  A default instance (using env-vars) is
        created if not provided.
    decoder:
        ``TransactionDecoder`` instance.  A default instance is created if
        not provided.
    """

    def __init__(
        self,
        log_path: str = _DEFAULT_LOG_PATH,
        summariser: LLMSummariser | None = None,
        decoder: TransactionDecoder | None = None,
    ) -> None:
        self._log_path = Path(log_path)
        self._log_path.parent.mkdir(parents=True, exist_ok=True)
        self._summariser = summariser or LLMSummariser()
        self._decoder = decoder or TransactionDecoder()

    def log_event(self, event: dict[str, Any]) -> TxEvent:
        """
        Decode *event*, generate an LLM summary, persist to JSONL, and return
        the ``TxEvent``.
        """
        tx_event = self._decoder.decode(event)
        tx_event.summary = self._summariser.summarise(tx_event)
        self._persist(tx_event)
        logger.info("[%s] %s", tx_event.event_name, tx_event.summary)
        return tx_event

    def log_events(self, events: list[dict[str, Any]]) -> list[TxEvent]:
        """Batch version of :meth:`log_event`."""
        return [self.log_event(e) for e in events]

    def _persist(self, tx_event: TxEvent) -> None:
        record = asdict(tx_event)
        # Convert bytes values to hex strings for JSON serialisation
        record = _jsonify(record)
        with self._log_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(record, ensure_ascii=False) + "\n")

    # ── Query helpers ────────────────────────────────────────────────────

    def recent(self, n: int = 20) -> list[dict[str, Any]]:
        """Return the *n* most-recently logged entries."""
        if not self._log_path.is_file():
            return []
        lines = self._log_path.read_text(encoding="utf-8").splitlines()
        return [json.loads(line) for line in lines[-n:]]

    def by_event(self, event_name: str) -> list[dict[str, Any]]:
        """Return all logged entries for a specific event type."""
        if not self._log_path.is_file():
            return []
        results = []
        for line in self._log_path.read_text(encoding="utf-8").splitlines():
            entry = json.loads(line)
            if entry.get("event_name") == event_name:
                results.append(entry)
        return results

    def by_address(self, address: str) -> list[dict[str, Any]]:
        """Return all logged entries involving *address* as from/to/claimant."""
        address_lc = address.lower()
        if not self._log_path.is_file():
            return []
        results = []
        for line in self._log_path.read_text(encoding="utf-8").splitlines():
            entry = json.loads(line)
            if any(
                str(entry.get(f, "")).lower() == address_lc
                for f in ("from_addr", "to_addr", "claimant", "new_validator")
            ):
                results.append(entry)
        return results

    def stats(self) -> dict[str, Any]:
        """
        Return aggregate statistics across all persisted log entries.

        Returns a dict with total event counts per type and total FLSH
        transferred / claimed.
        """
        if not self._log_path.is_file():
            return {}
        counts: dict[str, int] = {}
        total_transferred = 0.0
        total_claimed = 0.0
        for line in self._log_path.read_text(encoding="utf-8").splitlines():
            entry = json.loads(line)
            name = entry.get("event_name", "Unknown")
            counts[name] = counts.get(name, 0) + 1
            if name in ("TapAndGo", "PosCheckout", "Transfer"):
                total_transferred += float(entry.get("amount_flsh", 0))
            elif name == "WhitelistClaimed":
                total_claimed += float(entry.get("amount_flsh", 0))
        return {
            "event_counts": counts,
            "total_transferred_flsh": round(total_transferred, 4),
            "total_claimed_flsh": round(total_claimed, 4),
            "total_events": sum(counts.values()),
        }


# ---------------------------------------------------------------------------
# Private helpers
# ---------------------------------------------------------------------------


def _wei_to_flsh(amount_wei: int | str) -> float:
    try:
        return int(amount_wei) / _ONE_FLSH
    except (TypeError, ValueError):
        return 0.0


def _hex_or_str(value: Any) -> str:
    if isinstance(value, (bytes, bytearray)):
        return "0x" + value.hex()
    if isinstance(value, memoryview):
        return "0x" + bytes(value).hex()
    return str(value)


def _abbrev(addr: str) -> str:
    """Abbreviate an Ethereum address to 0xABCD…1234 form."""
    if not addr or len(addr) < 10:
        return addr or ""
    # Strip 0x prefix for processing
    clean = addr[2:] if addr.startswith("0x") else addr
    if len(clean) >= 8:
        return f"0x{clean[:6]}…{clean[-4:]}"
    return addr


def _jsonify(obj: Any) -> Any:
    """Recursively convert bytes to hex strings so the record is JSON-safe."""
    if isinstance(obj, (bytes, bytearray)):
        return "0x" + obj.hex()
    if isinstance(obj, dict):
        return {k: _jsonify(v) for k, v in obj.items()}
    if isinstance(obj, list):
        return [_jsonify(v) for v in obj]
    return obj
