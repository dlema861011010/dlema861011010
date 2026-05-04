"""
blockscout.py — Async Blockscout v2 API client for the FuseSpark testnet.

Wraps the Blockscout REST API exposed at explorer.fusespark.io.
Used by the FuseFlash bridge service to relay on-chain state to the
Go node and the Ember frontend.
"""

import asyncio
from typing import Any, Dict, List, Optional

import httpx

BLOCKSCOUT_BASE = "https://explorer.fusespark.io/api/v2"
DEFAULT_TIMEOUT = 15.0  # seconds


class BlockscoutClient:
    """Thin async client around the Blockscout v2 REST API."""

    def __init__(
        self,
        base_url: str = BLOCKSCOUT_BASE,
        timeout: float = DEFAULT_TIMEOUT,
    ) -> None:
        self._base = base_url.rstrip("/")
        self._client = httpx.AsyncClient(
            base_url=self._base,
            timeout=timeout,
            headers={"Accept": "application/json"},
        )

    async def close(self) -> None:
        await self._client.aclose()

    async def __aenter__(self) -> "BlockscoutClient":
        return self

    async def __aexit__(self, *_: Any) -> None:
        await self.close()

    # ── Core helper ──────────────────────────────────────────

    async def _get(self, path: str, params: Optional[Dict] = None) -> Any:
        resp = await self._client.get(path, params=params or {})
        resp.raise_for_status()
        return resp.json()

    # ── Addresses ────────────────────────────────────────────

    async def get_address(self, address: str) -> Dict:
        """Return address metadata: balance, tx count, token balances."""
        return await self._get(f"/addresses/{address}")

    async def get_transactions(
        self,
        address: str,
        page: int = 1,
        limit: int = 20,
    ) -> Dict:
        """Return paginated transaction list for an address."""
        return await self._get(
            f"/addresses/{address}/transactions",
            params={"page": page, "limit": limit},
        )

    async def get_token_transfers(
        self,
        address: str,
        page: int = 1,
        limit: int = 20,
    ) -> Dict:
        """Return ERC-20/ERC-721 token transfers for an address."""
        return await self._get(
            f"/addresses/{address}/token-transfers",
            params={"page": page, "limit": limit},
        )

    # ── Transactions ─────────────────────────────────────────

    async def get_transaction(self, tx_hash: str) -> Dict:
        """Return full transaction details by hash."""
        return await self._get(f"/transactions/{tx_hash}")

    # ── Blocks ───────────────────────────────────────────────

    async def get_block(self, number_or_tag: str = "latest") -> Dict:
        """Return block details. Pass 'latest' or a decimal block number."""
        return await self._get(f"/blocks/{number_or_tag}")

    async def get_latest_block_number(self) -> int:
        """Return the current head block number."""
        block = await self.get_block("latest")
        return int(block.get("height") or block.get("number") or 0)

    # ── Search ────────────────────────────────────────────────

    async def search(self, query: str) -> Dict:
        """Search by address, tx hash, block number, or token."""
        return await self._get("/search", params={"q": query})

    # ── Stats ─────────────────────────────────────────────────

    async def get_stats(self) -> Dict:
        """Return network stats (avg gas price, total tx count, etc.)."""
        return await self._get("/stats")

    # ── Convenience URLs ─────────────────────────────────────

    @staticmethod
    def explorer_url(entity_type: str, value: str) -> str:
        """Build a Blockscout explorer URL."""
        base = "https://explorer.fusespark.io"
        mapping = {"tx": "tx", "address": "address", "block": "block", "token": "token"}
        slug = mapping.get(entity_type, entity_type)
        return f"{base}/{slug}/{value}"


# ── CLI smoke test ────────────────────────────────────────────────────────────

async def _main() -> None:
    async with BlockscoutClient() as client:
        try:
            stats = await client.get_stats()
            print("Network stats:", stats)
            block = await client.get_latest_block_number()
            print(f"Latest block: {block}")
        except httpx.HTTPError as exc:
            print(f"HTTP error: {exc}")


if __name__ == "__main__":
    asyncio.run(_main())
