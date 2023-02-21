/* eslint valid-jsdoc: "off" */

'use strict';

/**
 * @param {Egg.EggAppInfo} appInfo app info
 */
module.exports = appInfo => {
  /**
   * built-in config
   * @type {Egg.EggAppConfig}
   **/
  const config = exports = {};

  // use for cookie sign key, should change to your own and keep security
  config.keys = appInfo.name + '_1676871960851_478';

  // add your middleware config here
  config.middleware = [];

  config.view = {
      defaultViewEngine: 'nunjucks',
  }

  config.io = {
      init: {}, // passed to engine.io
      namespace: {
          '/': {
              connectionMiddleware: ['connection'],
              packetMiddleware: [],
          },
      },
      redis: {
          host: '127.0.0.1',
          port: 6379,
      },
  }

  // add your user config here
  const userConfig = {
    // myAppName: 'egg',
  };

  return {
    ...config,
    ...userConfig,
  };
};
