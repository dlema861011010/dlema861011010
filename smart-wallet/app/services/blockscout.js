import Service from '@ember/service';
import { tracked } from '@glimmer/tracking';

const BLOCKSCOUT_BASE = 'https://explorer.fusespark.io/api/v2';

export default class BlockscoutService extends Service {
  @tracked latestBlock = null;
  @tracked networkStats = null;

  _cache = new Map();

  async _fetch(path, params = {}) {
    const url = new URL(`${BLOCKSCOUT_BASE}${path}`);
    Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
    const key = url.toString();
    if (this._cache.has(key)) return this._cache.get(key);

    const res = await fetch(url, {
      headers: { Accept: 'application/json' },
    });
    if (!res.ok) throw new Error(`Blockscout API error ${res.status}: ${path}`);
    const data = await res.json();
    this._cache.set(key, data);
    // Expire cache after 30 s.
    setTimeout(() => this._cache.delete(key), 30_000);
    return data;
  }

  /** Fetch address summary (balance, tx count, token balances). */
  async getAddress(address) {
    return this._fetch(`/addresses/${address}`);
  }

  /** Fetch transaction list for an address. */
  async getTransactions(address, { page = 1, limit = 20 } = {}) {
    return this._fetch(`/addresses/${address}/transactions`, { page, limit });
  }

  /** Fetch a single transaction by hash. */
  async getTransaction(hash) {
    return this._fetch(`/transactions/${hash}`);
  }

  /** Fetch a block by number or "latest". */
  async getBlock(numberOrTag = 'latest') {
    return this._fetch(`/blocks/${numberOrTag}`);
  }

  /** Fetch token transfers for an address. */
  async getTokenTransfers(address, { page = 1, limit = 20 } = {}) {
    return this._fetch(`/addresses/${address}/token-transfers`, { page, limit });
  }

  /** Search by address, tx hash, or block number. */
  async search(query) {
    return this._fetch('/search', { q: query });
  }

  /** Fetch stats (average gas price, tx count, etc.). */
  async getStats() {
    const data = await this._fetch('/stats');
    this.networkStats = data;
    return data;
  }

  /** Refresh the latest block number. */
  async refreshLatestBlock() {
    const data = await this.getBlock('latest');
    this.latestBlock = data?.height ?? data?.number ?? null;
    return this.latestBlock;
  }

  /** Build a Blockscout explorer URL for any entity. */
  url(type, value) {
    const base = 'https://explorer.fusespark.io';
    const map = { tx: 'tx', address: 'address', block: 'block', token: 'token' };
    return `${base}/${map[type] ?? type}/${value}`;
  }
}
