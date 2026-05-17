"""Tests for bridge/llm_logger.py – no LLM API key needed."""
from __future__ import annotations

import json
import os
import sys
import tempfile
from pathlib import Path

import pytest

# Add bridge/ to sys.path so imports resolve without installation
sys.path.insert(0, str(Path(__file__).parent.parent))


# ---------------------------------------------------------------------------
# TransactionDecoder
# ---------------------------------------------------------------------------


class TestTransactionDecoder:
    def setup_method(self):
        from llm_logger import TransactionDecoder

        self.dec = TransactionDecoder()

    def _tap_event(self) -> dict:
        return {
            "event": "TapAndGo",
            "transactionHash": bytes.fromhex("ab" * 32),
            "blockNumber": 42,
            "args": {
                "from": "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
                "to": "0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
                "amount": 1_500_000_000_000_000_000,  # 1.5 FLSH in wei
                "nonce": b"\x01" * 32,
            },
        }

    def test_tap_and_go_decoded(self):
        ev = self.dec.decode(self._tap_event())
        assert ev.event_name == "TapAndGo"
        assert ev.block_number == 42
        assert ev.amount_flsh == pytest.approx(1.5)
        assert ev.from_addr == "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
        assert ev.to_addr == "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"

    def test_pos_checkout_decoded(self):
        ev = self.dec.decode(
            {
                "event": "PosCheckout",
                "transactionHash": b"\xca" * 32,
                "blockNumber": 100,
                "args": {
                    "payer": "0xAAA",
                    "merchant": "0xBBB",
                    "amount": 5 * 10**18,
                    "sessionId": b"\xff" * 32,
                },
            }
        )
        assert ev.event_name == "PosCheckout"
        assert ev.amount_flsh == pytest.approx(5.0)
        assert ev.from_addr == "0xAAA"
        assert ev.to_addr == "0xBBB"

    def test_whitelist_claimed_decoded(self):
        ev = self.dec.decode(
            {
                "event": "WhitelistClaimed",
                "transactionHash": b"\x00" * 32,
                "blockNumber": 7,
                "args": {
                    "claimant": "0xCCC",
                    "amount": 100 * 10**18,
                },
            }
        )
        assert ev.event_name == "WhitelistClaimed"
        assert ev.claimant == "0xCCC"
        assert ev.amount_flsh == pytest.approx(100.0)

    def test_unknown_event(self):
        ev = self.dec.decode(
            {
                "event": "Approval",
                "transactionHash": b"\x00" * 32,
                "blockNumber": 1,
                "args": {},
            }
        )
        assert ev.event_name == "Approval"


# ---------------------------------------------------------------------------
# LLMSummariser (rule-based path – no API key required)
# ---------------------------------------------------------------------------


class TestLLMSummariserRuleBased:
    def setup_method(self):
        os.environ.pop("OPENAI_API_KEY", None)
        from llm_logger import LLMSummariser

        self.summariser = LLMSummariser()

    def _make_event(self, **kwargs):
        from llm_logger import TxEvent

        defaults = dict(
            event_name="TapAndGo",
            tx_hash="0x" + "ab" * 32,
            block_number=1,
            timestamp="2026-01-01T00:00:00+00:00",
            from_addr="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
            to_addr="0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
            amount_flsh=1.5,
            nonce="0x" + "01" * 32,
        )
        defaults.update(kwargs)
        return TxEvent(**defaults)

    def test_tap_and_go_summary(self):
        ev = self._make_event()
        summary = self.summariser.summarise(ev)
        assert "Tap-and-Go" in summary
        assert "1.5000 FLSH" in summary

    def test_pos_checkout_summary(self):
        ev = self._make_event(
            event_name="PosCheckout",
            nonce="",
            session_id="0x" + "ff" * 32,
        )
        summary = self.summariser.summarise(ev)
        assert "POS Checkout" in summary

    def test_whitelist_claimed_summary(self):
        ev = self._make_event(
            event_name="WhitelistClaimed",
            claimant="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
            nonce="",
            from_addr="",
            to_addr="",
        )
        summary = self.summariser.summarise(ev)
        assert "Whitelist" in summary

    def test_unknown_event_summary(self):
        ev = self._make_event(event_name="Approval", nonce="", from_addr="", to_addr="")
        summary = self.summariser.summarise(ev)
        assert "Approval" in summary or "event" in summary.lower()


# ---------------------------------------------------------------------------
# TransactionLogger (no LLM)
# ---------------------------------------------------------------------------


class TestTransactionLogger:
    def setup_method(self):
        os.environ.pop("OPENAI_API_KEY", None)

    def _make_logger(self, tmp_path):
        from llm_logger import TransactionLogger

        return TransactionLogger(log_path=str(tmp_path / "test.jsonl"))

    def _tap_event(self, block: int = 1) -> dict:
        return {
            "event": "TapAndGo",
            "transactionHash": bytes([block % 256]) * 32,
            "blockNumber": block,
            "args": {
                "from": "0xAAA",
                "to": "0xBBB",
                "amount": 2 * 10**18,
                "nonce": b"\x00" * 32,
            },
        }

    def test_log_event_returns_tx_event(self, tmp_path):
        tx_logger = self._make_logger(tmp_path)
        result = tx_logger.log_event(self._tap_event())
        assert result.event_name == "TapAndGo"
        assert result.amount_flsh == pytest.approx(2.0)
        assert result.summary != ""

    def test_recent_returns_logged_entries(self, tmp_path):
        tx_logger = self._make_logger(tmp_path)
        for i in range(1, 4):
            tx_logger.log_event(self._tap_event(block=i))
        recent = tx_logger.recent(n=2)
        assert len(recent) == 2
        assert recent[-1]["block_number"] == 3

    def test_by_event_filters(self, tmp_path):
        tx_logger = self._make_logger(tmp_path)
        tx_logger.log_event(self._tap_event())
        tx_logger.log_event(
            {
                "event": "PosCheckout",
                "transactionHash": b"\xcc" * 32,
                "blockNumber": 5,
                "args": {"payer": "0xCCC", "merchant": "0xDDD", "amount": 10**18, "sessionId": b"\x00" * 32},
            }
        )
        taps = tx_logger.by_event("TapAndGo")
        pos = tx_logger.by_event("PosCheckout")
        assert len(taps) == 1
        assert len(pos) == 1

    def test_stats(self, tmp_path):
        tx_logger = self._make_logger(tmp_path)
        tx_logger.log_event(self._tap_event(block=1))
        tx_logger.log_event(self._tap_event(block=2))
        stats = tx_logger.stats()
        assert stats["event_counts"]["TapAndGo"] == 2
        assert stats["total_transferred_flsh"] == pytest.approx(4.0)
        assert stats["total_events"] == 2

    def test_by_address(self, tmp_path):
        tx_logger = self._make_logger(tmp_path)
        tx_logger.log_event(self._tap_event())
        results = tx_logger.by_address("0xAAA")
        assert len(results) == 1

    def test_empty_log_returns_empty(self, tmp_path):
        tx_logger = self._make_logger(tmp_path)
        assert tx_logger.recent() == []
        assert tx_logger.stats() == {}

    def test_jsonl_format(self, tmp_path):
        """Each line in the log file should be valid JSON."""
        tx_logger = self._make_logger(tmp_path)
        tx_logger.log_event(self._tap_event())
        lines = (tmp_path / "test.jsonl").read_text().strip().splitlines()
        assert len(lines) == 1
        record = json.loads(lines[0])
        assert record["event_name"] == "TapAndGo"
        assert "summary" in record
