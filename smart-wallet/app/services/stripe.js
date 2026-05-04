import Service from '@ember/service';
import { tracked } from '@glimmer/tracking';
import { action } from '@ember/object';
import { loadStripe } from '@stripe/stripe-js';

const PUBLISHABLE_KEY = import.meta.env?.VITE_STRIPE_PUBLISHABLE_KEY
  ?? window.STRIPE_PUBLISHABLE_KEY
  ?? 'pk_test_placeholder';

export default class StripeService extends Service {
  @tracked stripe = null;
  @tracked isLoaded = false;

  async load() {
    if (this.stripe) return this.stripe;
    this.stripe = await loadStripe(PUBLISHABLE_KEY);
    this.isLoaded = true;
    return this.stripe;
  }

  /**
   * Redirects to a Stripe Checkout session.
   * @param {string} sessionId  Checkout session ID from your backend
   */
  @action
  async redirectToCheckout(sessionId) {
    const stripe = await this.load();
    const { error } = await stripe.redirectToCheckout({ sessionId });
    if (error) throw new Error(error.message);
  }

  /**
   * Confirms a PaymentIntent to bridge fiat → SPARK top-up.
   * @param {string} clientSecret  PaymentIntent client secret
   * @param {string} billingName
   * @returns {Promise<{status: string}>}
   */
  @action
  async confirmPayment(clientSecret, billingName) {
    const stripe = await this.load();
    const { paymentIntent, error } = await stripe.confirmCardPayment(clientSecret, {
      payment_method: {
        card: this._cardElement,
        billing_details: { name: billingName },
      },
    });
    if (error) throw new Error(error.message);
    return { status: paymentIntent.status };
  }

  /**
   * Mounts a Stripe card element into a DOM node.
   * @param {Element} container
   */
  async mountCardElement(container) {
    const stripe = await this.load();
    const elements = stripe.elements();
    this._cardElement = elements.create('card', {
      style: {
        base: {
          color: '#1a1a2e',
          fontFamily: '"Inter", sans-serif',
          fontSize: '16px',
          '::placeholder': { color: '#a0aec0' },
        },
        invalid: { color: '#e53e3e' },
      },
    });
    this._cardElement.mount(container);
    return this._cardElement;
  }

  destroyCardElement() {
    if (this._cardElement) {
      this._cardElement.destroy();
      this._cardElement = null;
    }
  }
}
