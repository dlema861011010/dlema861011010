require("@nomicfoundation/hardhat-toolbox");

// Force WASM platform so Hardhat uses the bundled soljson.js
process.env.HARDHAT_DISABLE_TELEMETRY_PROMPT = "true";

/** @type import('hardhat/config').HardhatUserConfig */
module.exports = {
  solidity: {
    version: "0.8.26",
    settings: {
      optimizer: {
        enabled: true,
        runs: 200,
      },
    },
  },
  networks: {
    hardhat: {},
  },
  paths: {
    cache: "./cache",
    artifacts: "./artifacts",
  },
};
