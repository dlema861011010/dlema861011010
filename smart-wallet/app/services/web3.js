import Service from '@ember/service';
import { tracked } from '@glimmer/tracking';
import { action } from '@ember/object';
import { BrowserProvider, JsonRpcProvider, formatEther, parseEther } from 'ethers';

// FuseSpark Testnet (Chain ID 123)
const FUSESPARK_CHAIN = {
  chainId: '0x7B', // 123 in hex
  chainName: 'FuseSpark Testnet',
  nativeCurrency: { name: 'Spark', symbol: 'SPARK', decimals: 18 },
  rpcUrls: ['https://rpc.fusespark.io'],
  blockExplorerUrls: ['https://explorer.fusespark.io'],
};

export default class Web3Service extends Service {
  @tracked provider = null;
  @tracked signer = null;
  @tracked account = null;
  @tracked balance = null;
  @tracked chainId = null;
  @tracked isConnected = false;
  @tracked isConnecting = false;
  @tracked error = null;

  /** Read-only provider for the FuseSpark testnet (no wallet needed). */
  get readProvider() {
    return new JsonRpcProvider('https://rpc.fusespark.io');
  }

  /** True when the connected chain is FuseSpark testnet. */
  get isCorrectChain() {
    return this.chainId === 123;
  }

  /** Formatted balance string with 4 dp. */
  get formattedBalance() {
    if (this.balance === null) return '—';
    return parseFloat(formatEther(this.balance)).toFixed(4) + ' SPARK';
  }

  /** Abbreviated wallet address for display. */
  get shortAddress() {
    if (!this.account) return '';
    return `${this.account.slice(0, 6)}…${this.account.slice(-4)}`;
  }

  /** Blockscout explorer URL for the current account. */
  get explorerAddressUrl() {
    if (!this.account) return '#';
    return `https://explorer.fusespark.io/address/${this.account}`;
  }

  @action
  async connect() {
    this.isConnecting = true;
    this.error = null;
    try {
      if (!window.ethereum) {
        throw new Error('No Web3 wallet detected. Install MetaMask or a compatible wallet.');
      }
      this.provider = new BrowserProvider(window.ethereum);
      await this.provider.send('eth_requestAccounts', []);
      this.signer = await this.provider.getSigner();
      this.account = await this.signer.getAddress();
      const network = await this.provider.getNetwork();
      this.chainId = Number(network.chainId);

      if (!this.isCorrectChain) {
        await this._switchToFuseSpark();
      }
      await this.refreshBalance();
      this.isConnected = true;
      this._watchAccountChanges();
    } catch (err) {
      this.error = err.message;
      this.isConnected = false;
    } finally {
      this.isConnecting = false;
    }
  }

  @action
  async disconnect() {
    this.provider = null;
    this.signer = null;
    this.account = null;
    this.balance = null;
    this.chainId = null;
    this.isConnected = false;
  }

  @action
  async refreshBalance() {
    if (!this.provider || !this.account) return;
    this.balance = await this.provider.getBalance(this.account);
  }

  /**
   * Sends an EVM transaction on the FuseSpark testnet.
   * @param {string} to  recipient address
   * @param {string} amount  amount in SPARK (human-readable, e.g. "0.5")
   * @returns {Promise<string>} transaction hash
   */
  async sendTransaction(to, amount) {
    if (!this.signer) throw new Error('Wallet not connected');
    const tx = await this.signer.sendTransaction({
      to,
      value: parseEther(amount),
    });
    await tx.wait();
    await this.refreshBalance();
    return tx.hash;
  }

  /**
   * Signs a message with the connected wallet.
   * @param {string} message
   * @returns {Promise<string>} signature
   */
  async signMessage(message) {
    if (!this.signer) throw new Error('Wallet not connected');
    return this.signer.signMessage(message);
  }

  // --- private ---

  async _switchToFuseSpark() {
    try {
      await window.ethereum.request({
        method: 'wallet_switchEthereumChain',
        params: [{ chainId: FUSESPARK_CHAIN.chainId }],
      });
    } catch (switchError) {
      if (switchError.code === 4902) {
        await window.ethereum.request({
          method: 'wallet_addEthereumChain',
          params: [FUSESPARK_CHAIN],
        });
      } else {
        throw switchError;
      }
    }
    const network = await this.provider.getNetwork();
    this.chainId = Number(network.chainId);
  }

  _watchAccountChanges() {
    if (!window.ethereum) return;
    window.ethereum.on('accountsChanged', async (accounts) => {
      if (accounts.length === 0) {
        await this.disconnect();
      } else {
        this.account = accounts[0];
        await this.refreshBalance();
      }
    });
    window.ethereum.on('chainChanged', () => {
      window.location.reload();
    });
  }
}
