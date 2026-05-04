'use strict';

const EmberApp = require('ember-cli/lib/broccoli/ember-app');

module.exports = function (defaults) {
  const { Webpack } = require('@embroider/webpack');
  return require('@embroider/compat').compatBuild(defaults, Webpack, {
    staticAddonTestSupportFiles: true,
    staticAddonTrees: true,
    staticHelpers: true,
    staticModifiers: true,
    staticComponents: true,
    splitAtRoutes: ['wallet', 'explorer', 'bridge'],
    packagerOptions: {
      webpackConfig: {
        resolve: {
          fallback: {
            buffer: require.resolve('buffer/'),
            crypto: false,
            stream: false,
            assert: false,
            http: false,
            https: false,
            os: false,
            url: false,
          },
        },
      },
    },
  });
};
