import EmberRouter from '@ember/routing/router';
import config from 'fuseflash-smart-wallet/config/environment';

export default class Router extends EmberRouter {
  location = config.locationType;
  rootURL = config.rootURL;
}

Router.map(function () {
  this.route('wallet', function () {
    this.route('send');
    this.route('receive');
    this.route('history');
  });
  this.route('explorer', function () {
    this.route('transaction', { path: '/tx/:hash' });
    this.route('address', { path: '/address/:addr' });
    this.route('block', { path: '/block/:number' });
  });
  this.route('bridge');
  this.route('not-found', { path: '/*path' });
});
