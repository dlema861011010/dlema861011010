import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { action } from '@ember/object';
import { service } from '@ember/service';

export default class WalletConnectComponent extends Component {
  @service web3;

  @action
  connect() {
    return this.web3.connect();
  }

  @action
  disconnect() {
    return this.web3.disconnect();
  }
}
