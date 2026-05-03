"use strict";

/**
 * tapValidator.js
 *
 * Express backend that:
 *  - Maintains a Merkle whitelist for FLSH token claims.
 *  - Signs Tap-and-Go P2P transfer intents with an ECDSA key.
 *
 * Environment variables:
 *   TAP_VALIDATOR_PRIVATE_KEY  – hex private key of the tap-validator signer
 *   PORT                       – HTTP port (default 3000)
 */

const express = require("express");
const { ethers } = require("ethers");
const { MerkleTree } = require("merkletreejs");
const keccak256 = require("keccak256");

// ─── Validator signer ────────────────────────────────────────────────────────

const PRIVATE_KEY =
  process.env.TAP_VALIDATOR_PRIVATE_KEY ||
  // Default insecure dev key – never use in production
  "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80";

const validatorWallet = new ethers.Wallet(PRIVATE_KEY);

// ─── Whitelist state ─────────────────────────────────────────────────────────

/**
 * In-memory map of address → amount (as string to preserve precision).
 * Production deployments should persist this in a database.
 */
const whitelistEntries = new Map(); // address (lowercase) → amount (string)

let merkleTree = null;
let merkleRoot = ethers.ZeroHash;

/**
 * Rebuild the Merkle tree from current whitelist entries.
 */
function rebuildTree() {
  const leaves = Array.from(whitelistEntries.entries()).map(([addr, amount]) =>
    ethers.solidityPackedKeccak256(["address", "uint256"], [addr, amount])
  );

  if (leaves.length === 0) {
    merkleTree = null;
    merkleRoot = ethers.ZeroHash;
    return;
  }

  merkleTree = new MerkleTree(leaves, keccak256, { sortPairs: true });
  merkleRoot = "0x" + merkleTree.getRoot().toString("hex");
}

// ─── Express app ─────────────────────────────────────────────────────────────

const app = express();
app.use(express.json());

/**
 * POST /whitelist/add
 *
 * Body: { address: string, amount: string }
 * Response: { merkleRoot: string }
 */
app.post("/whitelist/add", (req, res) => {
  const { address, amount } = req.body;

  if (!address || !amount) {
    return res.status(400).json({ error: "address and amount are required" });
  }

  let checksumAddress;
  try {
    checksumAddress = ethers.getAddress(address);
  } catch {
    return res.status(400).json({ error: "invalid address" });
  }

  if (isNaN(Number(amount)) || BigInt(amount) <= 0n) {
    return res.status(400).json({ error: "amount must be a positive integer string" });
  }

  whitelistEntries.set(checksumAddress.toLowerCase(), amount);
  rebuildTree();

  return res.json({ merkleRoot });
});

/**
 * GET /whitelist/proof/:address/:amount
 *
 * Response: { proof: string[], merkleRoot: string }
 */
app.get("/whitelist/proof/:address/:amount", (req, res) => {
  const { address, amount } = req.params;

  let checksumAddress;
  try {
    checksumAddress = ethers.getAddress(address);
  } catch {
    return res.status(400).json({ error: "invalid address" });
  }

  if (!merkleTree) {
    return res.status(404).json({ error: "whitelist is empty" });
  }

  const leaf = ethers.solidityPackedKeccak256(
    ["address", "uint256"],
    [checksumAddress, amount]
  );

  const proof = merkleTree.getHexProof(leaf);

  if (proof.length === 0 && !merkleTree.verify(proof, leaf, merkleTree.getRoot())) {
    return res.status(404).json({ error: "address/amount not in whitelist" });
  }

  return res.json({ proof, merkleRoot });
});

/**
 * POST /tap/sign
 *
 * Body: { from: string, to: string, amount: string, nonce: string }
 * Response: { signature: string, intentHash: string }
 *
 * Signs keccak256(abi.encodePacked(from, to, amount, nonce)) using EIP-191
 * personal_sign prefix so the contract can recover with toEthSignedMessageHash.
 */
app.post("/tap/sign", async (req, res) => {
  const { from, to, amount, nonce } = req.body;

  if (!from || !to || !amount || !nonce) {
    return res.status(400).json({ error: "from, to, amount, and nonce are required" });
  }

  let fromAddress, toAddress;
  try {
    fromAddress = ethers.getAddress(from);
    toAddress = ethers.getAddress(to);
  } catch {
    return res.status(400).json({ error: "invalid address in from or to" });
  }

  // nonce must be a 32-byte hex string (bytes32)
  if (!/^0x[0-9a-fA-F]{64}$/.test(nonce)) {
    return res.status(400).json({ error: "nonce must be a 32-byte hex string (0x + 64 hex chars)" });
  }

  let amountBig;
  try {
    amountBig = BigInt(amount);
    if (amountBig <= 0n) throw new Error();
  } catch {
    return res.status(400).json({ error: "amount must be a positive integer string" });
  }

  // Replicate on-chain: keccak256(abi.encodePacked(from, to, amount, nonce))
  const intentHash = ethers.solidityPackedKeccak256(
    ["address", "address", "uint256", "bytes32"],
    [fromAddress, toAddress, amountBig, nonce]
  );

  // EIP-191 personal sign (matches ECDSA.toEthSignedMessageHash on-chain)
  const signature = await validatorWallet.signMessage(ethers.getBytes(intentHash));

  return res.json({ signature, intentHash });
});

// ─── Health check ─────────────────────────────────────────────────────────────

app.get("/health", (_req, res) => res.json({ status: "ok", validator: validatorWallet.address }));

// ─── Start ───────────────────────────────────────────────────────────────────

const PORT = process.env.PORT || 3000;

/* istanbul ignore next */
if (require.main === module) {
  app.listen(PORT, () => {
    console.log(`tapValidator running on port ${PORT}`);
    console.log(`Validator address: ${validatorWallet.address}`);
  });
}

module.exports = { app, validatorWallet, whitelistEntries, rebuildTree, getMerkleRoot: () => merkleRoot };
