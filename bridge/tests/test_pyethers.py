"""Tests for bridge/pyethers.py – no live RPC needed."""
from __future__ import annotations

import pytest

# ---------------------------------------------------------------------------
# sign_tap_intent
# ---------------------------------------------------------------------------


def test_sign_tap_intent_returns_65_bytes():
    """sign_tap_intent should return a 65-byte ECDSA signature."""
    from pyethers import sign_tap_intent

    # Hardhat account #0 private key (public test key, safe to commit)
    PRIVKEY = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
    FROM = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
    TO = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
    nonce = b"\x01" * 32

    sig = sign_tap_intent(PRIVKEY, FROM, TO, 1.0, nonce)
    assert isinstance(sig, bytes)
    assert len(sig) == 65


def test_sign_tap_intent_deterministic():
    """Same inputs should always produce the same signature."""
    from pyethers import sign_tap_intent

    PRIVKEY = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
    FROM = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
    TO = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
    nonce = b"\xde\xad\xbe\xef" + b"\x00" * 28

    sig1 = sign_tap_intent(PRIVKEY, FROM, TO, 2.5, nonce)
    sig2 = sign_tap_intent(PRIVKEY, FROM, TO, 2.5, nonce)
    assert sig1 == sig2


def test_sign_tap_intent_different_amounts_differ():
    """Different amounts should produce different signatures."""
    from pyethers import sign_tap_intent

    PRIVKEY = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
    FROM = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
    TO = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
    nonce = b"\xca\xfe" + b"\x00" * 30

    sig1 = sign_tap_intent(PRIVKEY, FROM, TO, 1.0, nonce)
    sig2 = sign_tap_intent(PRIVKEY, FROM, TO, 99.0, nonce)
    assert sig1 != sig2


# ---------------------------------------------------------------------------
# _to_wei / _from_wei helpers (imported via module internals)
# ---------------------------------------------------------------------------


def test_wei_round_trip():
    """1 FLSH should survive a to_wei → from_wei round-trip."""
    from pyethers import _to_wei, _from_wei

    for amount in [0.0, 1.0, 1.5, 1_000_000.0]:
        assert _from_wei(_to_wei(amount)) == pytest.approx(amount)


# ---------------------------------------------------------------------------
# FUSEFLASH_ABI sanity
# ---------------------------------------------------------------------------


def test_abi_contains_required_entries():
    """The embedded ABI should declare all required events and functions."""
    from pyethers import FUSEFLASH_ABI

    names = {e["name"] for e in FUSEFLASH_ABI if "name" in e}
    required = {
        "tapAndGo",
        "posCheckout",
        "claimWhitelist",
        "transfer",
        "balanceOf",
        "TapAndGo",
        "PosCheckout",
        "WhitelistClaimed",
        "Transfer",
    }
    assert required.issubset(names), f"Missing ABI entries: {required - names}"


# ---------------------------------------------------------------------------
# FuseFlashDeployer – connection error without live RPC
# ---------------------------------------------------------------------------


def test_deployer_raises_on_bad_rpc():
    """FuseFlashDeployer should raise ConnectionError for an unreachable RPC."""
    from pyethers import FuseFlashDeployer

    with pytest.raises(ConnectionError):
        FuseFlashDeployer(
            rpc_url="http://127.0.0.1:19999",  # nothing listening here
            private_key="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
        )


def test_client_raises_on_bad_rpc():
    """FuseFlashClient should raise ConnectionError for an unreachable RPC."""
    from pyethers import FuseFlashClient

    with pytest.raises(ConnectionError):
        FuseFlashClient(
            rpc_url="http://127.0.0.1:19999",
            contract_address="0x" + "a" * 40,
            private_key="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
        )
