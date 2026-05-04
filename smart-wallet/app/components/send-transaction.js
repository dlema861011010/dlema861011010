import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { action } from '@ember/object';
import { service } from '@ember/service';

export default class SendTransactionComponent extends Component {
  @service web3;

  @tracked recipient = '';
  @tracked amount = '';
  @tracked txHash = null;
  @tracked isSending = false;
  @tracked error = null;

  get isValid() {
    return (
      this.recipient.startsWith('0x') &&
      this.recipient.length === 42 &&
      parseFloat(this.amount) > 0
    );
  }

  @action
  updateRecipient(e) {
    this.recipient = e.target.value;
    this.error = null;
    this.txHash = null;
  }

  @action
  updateAmount(e) {
    this.amount = e.target.value;
    this.error = null;
    this.txHash = null;
  }

  @action
  async send() {
    if (!this.isValid) return;
    this.isSending = true;
    this.error = null;
    this.txHash = null;
    try {
      this.txHash = await this.web3.sendTransaction(this.recipient, this.amount);
      this.recipient = '';
      this.amount = '';
    } catch (err) {
      this.error = err.message;
    } finally {
      this.isSending = false;
    }
  }
}
