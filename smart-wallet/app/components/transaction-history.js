import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { action } from '@ember/object';
import { service } from '@ember/service';

export default class TransactionHistoryComponent extends Component {
  @service blockscout;
  @service web3;

  @tracked transactions = [];
  @tracked isLoading = false;
  @tracked error = null;
  @tracked page = 1;

  constructor(owner, args) {
    super(owner, args);
    if (this.web3.account) {
      this.load();
    }
  }

  @action
  async load() {
    if (!this.web3.account) return;
    this.isLoading = true;
    this.error = null;
    try {
      const data = await this.blockscout.getTransactions(this.web3.account, {
        page: this.page,
        limit: 20,
      });
      this.transactions = data?.items ?? data ?? [];
    } catch (err) {
      this.error = err.message;
    } finally {
      this.isLoading = false;
    }
  }

  @action
  async nextPage() {
    this.page += 1;
    await this.load();
  }

  @action
  async prevPage() {
    if (this.page > 1) {
      this.page -= 1;
      await this.load();
    }
  }

  txUrl(hash) {
    return this.blockscout.url('tx', hash);
  }

  shortHash(hash) {
    if (!hash) return '';
    return `${hash.slice(0, 10)}…${hash.slice(-8)}`;
  }
}
