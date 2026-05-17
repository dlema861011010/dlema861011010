"""
pyethers.py
===========
A Python ethers.js-style wrapper around web3.py for the FuseFlash ecosystem.

Provides:
  - FuseFlashDeployer  – compile (via stored ABI/bytecode) and deploy the
                         FuseFlashToken contract to any EVM-compatible network.
  - FuseFlashClient    – interact with an already-deployed contract:
                         * transfer, tapAndGo, posCheckout, claimWhitelist
                         * event subscription helpers
  - sign_tap_intent    – helper to produce the ECDSA signature expected by
                         tapAndGo / the off-chain tap-validator.

Usage example
-------------
    from pyethers import FuseFlashDeployer, FuseFlashClient, sign_tap_intent
    import os

    deployer = FuseFlashDeployer(rpc_url=os.getenv("RPC_URL"),
                                  private_key=os.getenv("DEPLOY_KEY"))
    contract_address = deployer.deploy(
        tap_validator="0xABC...",
        merkle_root=bytes(32),
    )

    client = FuseFlashClient(rpc_url=os.getenv("RPC_URL"),
                              contract_address=contract_address,
                              private_key=os.getenv("WALLET_KEY"))
    tx_hash = client.transfer(to="0xDEF...", amount_flsh=10.0)
"""

from __future__ import annotations

import json
import logging
import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from eth_account import Account
from eth_account.messages import encode_defunct
from web3 import Web3
from web3.contract import Contract
from web3.types import TxReceipt

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# ABI – the minimal subset of FuseFlashToken needed for deployment and
# interaction.  Generated from contracts/FuseFlashToken.sol (Solidity 0.8.20,
# OpenZeppelin 5.x).
# ---------------------------------------------------------------------------
FUSEFLASH_ABI: list[dict[str, Any]] = [
    # Constructor
    {
        "type": "constructor",
        "inputs": [
            {"name": "initialOwner", "type": "address"},
            {"name": "_tapValidator", "type": "address"},
            {"name": "_merkleRoot", "type": "bytes32"},
        ],
        "stateMutability": "nonpayable",
    },
    # ── View ──────────────────────────────────────────────────────────────
    {"type": "function", "name": "name", "inputs": [], "outputs": [{"type": "string"}], "stateMutability": "view"},
    {"type": "function", "name": "symbol", "inputs": [], "outputs": [{"type": "string"}], "stateMutability": "view"},
    {"type": "function", "name": "decimals", "inputs": [], "outputs": [{"type": "uint8"}], "stateMutability": "view"},
    {"type": "function", "name": "totalSupply", "inputs": [], "outputs": [{"type": "uint256"}], "stateMutability": "view"},
    {"type": "function", "name": "balanceOf", "inputs": [{"name": "account", "type": "address"}], "outputs": [{"type": "uint256"}], "stateMutability": "view"},
    {"type": "function", "name": "allowance", "inputs": [{"name": "owner", "type": "address"}, {"name": "spender", "type": "address"}], "outputs": [{"type": "uint256"}], "stateMutability": "view"},
    {"type": "function", "name": "owner", "inputs": [], "outputs": [{"name": "", "type": "address"}], "stateMutability": "view"},
    {"type": "function", "name": "tapValidator", "inputs": [], "outputs": [{"name": "", "type": "address"}], "stateMutability": "view"},
    {"type": "function", "name": "merkleRoot", "inputs": [], "outputs": [{"name": "", "type": "bytes32"}], "stateMutability": "view"},
    {"type": "function", "name": "whitelistClaimed", "inputs": [{"name": "", "type": "address"}], "outputs": [{"type": "bool"}], "stateMutability": "view"},
    {"type": "function", "name": "usedTapNonces", "inputs": [{"name": "", "type": "bytes32"}], "outputs": [{"type": "bool"}], "stateMutability": "view"},
    {"type": "function", "name": "TOTAL_SUPPLY", "inputs": [], "outputs": [{"type": "uint256"}], "stateMutability": "view"},
    # ── Mutative ──────────────────────────────────────────────────────────
    {
        "type": "function",
        "name": "transfer",
        "inputs": [{"name": "to", "type": "address"}, {"name": "value", "type": "uint256"}],
        "outputs": [{"type": "bool"}],
        "stateMutability": "nonpayable",
    },
    {
        "type": "function",
        "name": "approve",
        "inputs": [{"name": "spender", "type": "address"}, {"name": "value", "type": "uint256"}],
        "outputs": [{"type": "bool"}],
        "stateMutability": "nonpayable",
    },
    {
        "type": "function",
        "name": "transferFrom",
        "inputs": [
            {"name": "from", "type": "address"},
            {"name": "to", "type": "address"},
            {"name": "value", "type": "uint256"},
        ],
        "outputs": [{"type": "bool"}],
        "stateMutability": "nonpayable",
    },
    {
        "type": "function",
        "name": "setMerkleRoot",
        "inputs": [{"name": "_merkleRoot", "type": "bytes32"}],
        "outputs": [],
        "stateMutability": "nonpayable",
    },
    {
        "type": "function",
        "name": "setTapValidator",
        "inputs": [{"name": "_tapValidator", "type": "address"}],
        "outputs": [],
        "stateMutability": "nonpayable",
    },
    {
        "type": "function",
        "name": "claimWhitelist",
        "inputs": [
            {"name": "amount", "type": "uint256"},
            {"name": "proof", "type": "bytes32[]"},
        ],
        "outputs": [],
        "stateMutability": "nonpayable",
    },
    {
        "type": "function",
        "name": "tapAndGo",
        "inputs": [
            {"name": "from", "type": "address"},
            {"name": "to", "type": "address"},
            {"name": "amount", "type": "uint256"},
            {"name": "nonce", "type": "bytes32"},
            {"name": "signature", "type": "bytes"},
        ],
        "outputs": [],
        "stateMutability": "nonpayable",
    },
    {
        "type": "function",
        "name": "posCheckout",
        "inputs": [
            {"name": "merchant", "type": "address"},
            {"name": "amount", "type": "uint256"},
            {"name": "sessionId", "type": "bytes32"},
        ],
        "outputs": [],
        "stateMutability": "nonpayable",
    },
    # ── Events ────────────────────────────────────────────────────────────
    {
        "type": "event",
        "name": "Transfer",
        "inputs": [
            {"name": "from", "type": "address", "indexed": True},
            {"name": "to", "type": "address", "indexed": True},
            {"name": "value", "type": "uint256", "indexed": False},
        ],
        "anonymous": False,
    },
    {
        "type": "event",
        "name": "WhitelistClaimed",
        "inputs": [
            {"name": "claimant", "type": "address", "indexed": True},
            {"name": "amount", "type": "uint256", "indexed": False},
        ],
        "anonymous": False,
    },
    {
        "type": "event",
        "name": "TapAndGo",
        "inputs": [
            {"name": "from", "type": "address", "indexed": True},
            {"name": "to", "type": "address", "indexed": True},
            {"name": "amount", "type": "uint256", "indexed": False},
            {"name": "nonce", "type": "bytes32", "indexed": True},
        ],
        "anonymous": False,
    },
    {
        "type": "event",
        "name": "PosCheckout",
        "inputs": [
            {"name": "payer", "type": "address", "indexed": True},
            {"name": "merchant", "type": "address", "indexed": True},
            {"name": "amount", "type": "uint256", "indexed": False},
            {"name": "sessionId", "type": "bytes32", "indexed": True},
        ],
        "anonymous": False,
    },
    {
        "type": "event",
        "name": "MerkleRootUpdated",
        "inputs": [{"name": "newRoot", "type": "bytes32", "indexed": True}],
        "anonymous": False,
    },
    {
        "type": "event",
        "name": "TapValidatorUpdated",
        "inputs": [{"name": "newValidator", "type": "address", "indexed": True}],
        "anonymous": False,
    },
]

# Placeholder bytecode – replace with the output of `hardhat compile`
# (artifacts/contracts/FuseFlashToken.sol/FuseFlashToken.json → bytecode).
# When FUSEFLASH_BYTECODE_PATH env-var points to that JSON file, the deployer
# will load it automatically.
_BYTECODE_PLACEHOLDER = "0x"  # pragma: no cover

_DECIMALS = 18
_ONE_FLSH = 10**_DECIMALS


def _load_bytecode() -> str:
    """Load compiled bytecode from the Hardhat artifact if available."""
    env_path = os.getenv("FUSEFLASH_BYTECODE_PATH")
    if env_path:
        p = Path(env_path)
        if p.is_file():
            data = json.loads(p.read_text())
            return data.get("bytecode", _BYTECODE_PLACEHOLDER)
    # Fallback: look for the default Hardhat artifact location relative to
    # this file.
    candidate = (
        Path(__file__).parent.parent
        / "contracts"
        / "artifacts"
        / "contracts"
        / "FuseFlashToken.sol"
        / "FuseFlashToken.json"
    )
    if candidate.is_file():
        data = json.loads(candidate.read_text())
        return data.get("bytecode", _BYTECODE_PLACEHOLDER)
    return _BYTECODE_PLACEHOLDER


# ---------------------------------------------------------------------------
# Data-classes
# ---------------------------------------------------------------------------


@dataclass
class DeployReceipt:
    """Result of a successful contract deployment."""

    contract_address: str
    tx_hash: str
    block_number: int
    gas_used: int


@dataclass
class TxResult:
    """Result of a mutative contract call."""

    tx_hash: str
    block_number: int
    gas_used: int
    status: int  # 1 = success, 0 = reverted


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _to_wei(amount_flsh: float) -> int:
    """Convert a human-readable FLSH amount to base units (wei, 18 decimals)."""
    return int(amount_flsh * _ONE_FLSH)


def _from_wei(amount_wei: int) -> float:
    """Convert base-unit FLSH to a human-readable float."""
    return amount_wei / _ONE_FLSH


def sign_tap_intent(
    private_key: str,
    from_addr: str,
    to_addr: str,
    amount_flsh: float,
    nonce: bytes,
) -> bytes:
    """
    Produce the ECDSA signature expected by FuseFlashToken.tapAndGo.

    The contract verifies:
        keccak256(abi.encodePacked(from, to, amount, nonce))
    after wrapping with EIP-191 (eth_sign prefix).

    Parameters
    ----------
    private_key:
        Hex-encoded private key of the tap-validator account.
    from_addr, to_addr:
        Checksummed Ethereum addresses of payer and payee.
    amount_flsh:
        Human-readable token amount (e.g. 1.5 = 1.5 FLSH).
    nonce:
        32-byte unique session nonce.

    Returns
    -------
    bytes
        65-byte ECDSA signature (r || s || v).
    """
    w3 = Web3()
    from_addr = Web3.to_checksum_address(from_addr)
    to_addr = Web3.to_checksum_address(to_addr)
    amount_wei = _to_wei(amount_flsh)
    nonce_b32 = nonce if len(nonce) == 32 else nonce.ljust(32, b"\x00")

    # Matches abi.encodePacked(from, to, amount, nonce)
    packed = (
        Web3.to_bytes(hexstr=from_addr)
        + Web3.to_bytes(hexstr=to_addr)
        + amount_wei.to_bytes(32, "big")
        + nonce_b32
    )
    msg_hash = Web3.keccak(packed)
    signable = encode_defunct(msg_hash)
    signed = w3.eth.account.sign_message(signable, private_key=private_key)
    return signed.signature


# ---------------------------------------------------------------------------
# FuseFlashDeployer
# ---------------------------------------------------------------------------


class FuseFlashDeployer:
    """
    Deploy a new FuseFlashToken contract to any EVM-compatible network.

    Parameters
    ----------
    rpc_url:
        JSON-RPC endpoint (e.g. Hardhat local: ``http://127.0.0.1:8545``).
    private_key:
        Hex-encoded private key of the deployer / owner account.
    """

    def __init__(self, rpc_url: str, private_key: str) -> None:
        self._w3 = Web3(Web3.HTTPProvider(rpc_url))
        if not self._w3.is_connected():
            raise ConnectionError(f"Cannot connect to RPC at {rpc_url}")
        self._account = Account.from_key(private_key)
        self._w3.eth.default_account = self._account.address

    @property
    def address(self) -> str:
        """Deployer wallet address."""
        return self._account.address

    def deploy(
        self,
        tap_validator: str,
        merkle_root: bytes = bytes(32),
        gas_limit: int | None = None,
    ) -> DeployReceipt:
        """
        Deploy a new FuseFlashToken and return the deployment receipt.

        Parameters
        ----------
        tap_validator:
            Address of the off-chain tap-validator service.
        merkle_root:
            Initial 32-byte Merkle root for whitelist allocations.
            Defaults to all-zeros (can be set later with setMerkleRoot).
        gas_limit:
            Optional override; if omitted the transaction will be estimated.

        Returns
        -------
        DeployReceipt
        """
        bytecode = _load_bytecode()
        if bytecode == _BYTECODE_PLACEHOLDER:
            raise RuntimeError(
                "FuseFlashToken bytecode not found.  Run `cd contracts && npx hardhat compile` "
                "and set FUSEFLASH_BYTECODE_PATH or ensure the Hardhat artifact is present."
            )

        tap_validator = Web3.to_checksum_address(tap_validator)
        merkle_root_b32 = merkle_root[:32].ljust(32, b"\x00")

        factory = self._w3.eth.contract(abi=FUSEFLASH_ABI, bytecode=bytecode)
        constructor_tx = factory.constructor(
            self._account.address, tap_validator, merkle_root_b32
        )

        nonce = self._w3.eth.get_transaction_count(self._account.address)
        tx = constructor_tx.build_transaction(
            {
                "from": self._account.address,
                "nonce": nonce,
                "gas": gas_limit or constructor_tx.estimate_gas({"from": self._account.address}),
            }
        )
        signed = self._account.sign_transaction(tx)
        tx_hash = self._w3.eth.send_raw_transaction(signed.raw_transaction)
        receipt: TxReceipt = self._w3.eth.wait_for_transaction_receipt(tx_hash)

        addr = receipt["contractAddress"]
        logger.info("FuseFlashToken deployed at %s (block %s)", addr, receipt["blockNumber"])
        return DeployReceipt(
            contract_address=addr,
            tx_hash=receipt["transactionHash"].hex(),
            block_number=receipt["blockNumber"],
            gas_used=receipt["gasUsed"],
        )


# ---------------------------------------------------------------------------
# FuseFlashClient
# ---------------------------------------------------------------------------


class FuseFlashClient:
    """
    Interact with a deployed FuseFlashToken contract.

    Parameters
    ----------
    rpc_url:
        JSON-RPC endpoint.
    contract_address:
        Address of the deployed FuseFlashToken.
    private_key:
        Hex-encoded private key of the calling wallet.
    """

    def __init__(
        self, rpc_url: str, contract_address: str, private_key: str
    ) -> None:
        self._w3 = Web3(Web3.HTTPProvider(rpc_url))
        if not self._w3.is_connected():
            raise ConnectionError(f"Cannot connect to RPC at {rpc_url}")
        self._account = Account.from_key(private_key)
        self._w3.eth.default_account = self._account.address
        self._contract: Contract = self._w3.eth.contract(
            address=Web3.to_checksum_address(contract_address),
            abi=FUSEFLASH_ABI,
        )

    # ── Getters ──────────────────────────────────────────────────────────

    @property
    def wallet_address(self) -> str:
        return self._account.address

    def balance_of(self, address: str) -> float:
        """Return the FLSH balance of *address* in human-readable units."""
        raw = self._contract.functions.balanceOf(
            Web3.to_checksum_address(address)
        ).call()
        return _from_wei(raw)

    def total_supply(self) -> float:
        """Return the total FLSH supply in human-readable units."""
        return _from_wei(self._contract.functions.totalSupply().call())

    def tap_validator(self) -> str:
        """Return the current tap-validator address."""
        return self._contract.functions.tapValidator().call()

    def merkle_root(self) -> bytes:
        """Return the current Merkle root as 32 bytes."""
        return bytes(self._contract.functions.merkleRoot().call())

    def is_nonce_used(self, nonce: bytes) -> bool:
        """Return True if the given 32-byte tap/POS nonce has been consumed."""
        return self._contract.functions.usedTapNonces(nonce[:32].ljust(32, b"\x00")).call()

    def has_claimed_whitelist(self, address: str) -> bool:
        return self._contract.functions.whitelistClaimed(
            Web3.to_checksum_address(address)
        ).call()

    # ── Mutative ─────────────────────────────────────────────────────────

    def _send(self, fn, gas_limit: int | None = None) -> TxResult:
        """Build, sign, and broadcast a contract function call."""
        nonce = self._w3.eth.get_transaction_count(self._account.address)
        tx = fn.build_transaction(
            {
                "from": self._account.address,
                "nonce": nonce,
                "gas": gas_limit or fn.estimate_gas({"from": self._account.address}),
            }
        )
        signed = self._account.sign_transaction(tx)
        tx_hash = self._w3.eth.send_raw_transaction(signed.raw_transaction)
        receipt: TxReceipt = self._w3.eth.wait_for_transaction_receipt(tx_hash)
        return TxResult(
            tx_hash=receipt["transactionHash"].hex(),
            block_number=receipt["blockNumber"],
            gas_used=receipt["gasUsed"],
            status=receipt["status"],
        )

    def transfer(self, to: str, amount_flsh: float) -> TxResult:
        """Transfer *amount_flsh* FLSH from the wallet to *to*."""
        fn = self._contract.functions.transfer(
            Web3.to_checksum_address(to), _to_wei(amount_flsh)
        )
        result = self._send(fn)
        logger.info("transfer %s FLSH → %s  tx=%s", amount_flsh, to, result.tx_hash)
        return result

    def approve(self, spender: str, amount_flsh: float) -> TxResult:
        """Approve *spender* to spend *amount_flsh* FLSH on behalf of the wallet."""
        fn = self._contract.functions.approve(
            Web3.to_checksum_address(spender), _to_wei(amount_flsh)
        )
        return self._send(fn)

    def tap_and_go(
        self,
        from_addr: str,
        to_addr: str,
        amount_flsh: float,
        nonce: bytes,
        signature: bytes,
    ) -> TxResult:
        """
        Execute a Tap-and-Go P2P transfer.

        The *signature* must be produced by the tap-validator using
        :func:`sign_tap_intent`.
        """
        fn = self._contract.functions.tapAndGo(
            Web3.to_checksum_address(from_addr),
            Web3.to_checksum_address(to_addr),
            _to_wei(amount_flsh),
            nonce[:32].ljust(32, b"\x00"),
            signature,
        )
        result = self._send(fn)
        logger.info(
            "tapAndGo %s FLSH %s→%s  nonce=%s  tx=%s",
            amount_flsh, from_addr, to_addr, nonce.hex(), result.tx_hash,
        )
        return result

    def pos_checkout(
        self, merchant: str, amount_flsh: float, session_id: bytes
    ) -> TxResult:
        """
        Execute a QR-based POS payment.

        Parameters
        ----------
        merchant:    Merchant's receiving address.
        amount_flsh: Human-readable FLSH amount.
        session_id:  32-byte unique POS session identifier.
        """
        fn = self._contract.functions.posCheckout(
            Web3.to_checksum_address(merchant),
            _to_wei(amount_flsh),
            session_id[:32].ljust(32, b"\x00"),
        )
        result = self._send(fn)
        logger.info(
            "posCheckout %s FLSH → %s  session=%s  tx=%s",
            amount_flsh, merchant, session_id.hex(), result.tx_hash,
        )
        return result

    def claim_whitelist(self, amount_flsh: float, proof: list[bytes]) -> TxResult:
        """
        Claim a Merkle-whitelist allocation.

        Parameters
        ----------
        amount_flsh: Allocation amount in human-readable FLSH.
        proof:       Merkle proof as a list of 32-byte nodes.
        """
        proof_b32 = [p[:32].ljust(32, b"\x00") for p in proof]
        fn = self._contract.functions.claimWhitelist(_to_wei(amount_flsh), proof_b32)
        result = self._send(fn)
        logger.info(
            "claimWhitelist %s FLSH  tx=%s", amount_flsh, result.tx_hash
        )
        return result

    def set_tap_validator(self, new_validator: str) -> TxResult:
        """Update the tap-validator address (owner only)."""
        fn = self._contract.functions.setTapValidator(
            Web3.to_checksum_address(new_validator)
        )
        return self._send(fn)

    def set_merkle_root(self, new_root: bytes) -> TxResult:
        """Update the Merkle root (owner only)."""
        fn = self._contract.functions.setMerkleRoot(
            new_root[:32].ljust(32, b"\x00")
        )
        return self._send(fn)

    # ── Event helpers ────────────────────────────────────────────────────

    def get_tap_and_go_events(
        self, from_block: int = 0, to_block: int | str = "latest"
    ) -> list[dict[str, Any]]:
        """Fetch all TapAndGo events in the given block range."""
        event_filter = self._contract.events.TapAndGo.create_filter(
            from_block=from_block, to_block=to_block
        )
        return [dict(e) for e in event_filter.get_all_entries()]

    def get_pos_checkout_events(
        self, from_block: int = 0, to_block: int | str = "latest"
    ) -> list[dict[str, Any]]:
        """Fetch all PosCheckout events in the given block range."""
        event_filter = self._contract.events.PosCheckout.create_filter(
            from_block=from_block, to_block=to_block
        )
        return [dict(e) for e in event_filter.get_all_entries()]

    def get_whitelist_claimed_events(
        self, from_block: int = 0, to_block: int | str = "latest"
    ) -> list[dict[str, Any]]:
        """Fetch all WhitelistClaimed events in the given block range."""
        event_filter = self._contract.events.WhitelistClaimed.create_filter(
            from_block=from_block, to_block=to_block
        )
        return [dict(e) for e in event_filter.get_all_entries()]
