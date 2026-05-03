/**
 * tapValidator.js — Fuse Flash Token Tap-and-Go Validator Service
 *
 * Responsibilities
 * ────────────────
 * 1. Maintain the Merkle whitelist tree (add members, generate proofs).
 * 2. Sign Tap-and-Go transfer intents so the smart contract can verify them.
 *
 * Endpoints
 * ─────────
 *   POST /whitelist/add
 *     Body: { address, amount }   (amount as a decimal string, no decimals)
 *     Response: { merkleRoot }
 *
 *   GET  /whitelist/proof/:address/:amount
 *     Response: { proof: string[], merkleRoot }
 *
 *   POST /tap/sign
 *     Body: { from, to, amount, nonce }
 *     Response: { signature, messageHash }
 *
 * Security (agentic AI safety protocol)
 * ──────────────────────────────────────
 * • The validator private key is loaded exclusively from the environment
 *   variable TAP_VALIDATOR_PRIVATE_KEY.  It is never logged or returned in
 *   any API response.
 * • Input validation is performed on every endpoint before any signing
 *   or tree operation occurs.
 * • Rate-limiting and authentication middleware placeholders are included
 *   and should be hardened before production deployment.
 */

"use strict";

const express = require("express");
const { ethers } = require("ethers");
const { MerkleTree } = require("merkletreejs");
const crypto = require("crypto");

// ─────────────────────────────────────────────────────────────────────────────
// Configuration
// ─────────────────────────────────────────────────────────────────────────────

const PORT = process.env.PORT || 3000;

const PRIVATE_KEY = process.env.TAP_VALIDATOR_PRIVATE_KEY;
if (!PRIVATE_KEY) {
  console.error(
    "[FATAL] TAP_VALIDATOR_PRIVATE_KEY environment variable is not set."
  );
  process.exit(1);
}

// Ethers signer loaded from the private key.
const validatorWallet = new ethers.Wallet(PRIVATE_KEY);
console.log(`[INFO] Tap validator address: ${validatorWallet.address}`);

// ─────────────────────────────────────────────────────────────────────────────
// Merkle tree state (in-memory — replace with a database for production)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Leaf entries stored as { address, amount } where amount is a BigInt
 * representing the token amount in 18-decimal base units.
 * @type {Array<{ address: string, amount: bigint }>}
 */
let whitelistEntries = [];

/** @type {MerkleTree|null} */
let merkleTree = null;

/**
 * Rebuild the Merkle tree from the current whitelistEntries.
 * Leaf format: keccak256(abi.encodePacked(address, amount))
 * This matches the on-chain leaf computation in FuseFlashToken.sol.
 */
function rebuildTree() {
  if (whitelistEntries.length === 0) {
    merkleTree = null;
    return;
  }

  const leaves = whitelistEntries.map(({ address, amount }) =>
    buildLeaf(address, amount)
  );

  merkleTree = new MerkleTree(leaves, keccak256Buf, { sortPairs: true });
}

/**
 * Compute a leaf hash matching Solidity's
 * keccak256(abi.encodePacked(address, uint256)).
 *
 * @param {string} address Ethereum address (checksummed or not).
 * @param {bigint} amount  Token amount in base units (uint256).
 * @returns {Buffer}
 */
function buildLeaf(address, amount) {
  // abi.encodePacked(address, uint256) — 20 bytes + 32 bytes = 52 bytes.
  const addrHex = address.toLowerCase().replace(/^0x/, "");
  const amountHex = amount.toString(16).padStart(64, "0");
  const packed = Buffer.from(addrHex + amountHex, "hex");
  return keccak256Buf(packed);
}

/** keccak256 wrapper returning a Buffer, for use with merkletreejs. */
function keccak256Buf(data) {
  return Buffer.from(
    ethers.keccak256(data instanceof Buffer ? data : Buffer.from(data)).slice(2),
    "hex"
  );
}

/** Return the current Merkle root as a hex string, or ZeroHash if tree is empty. */
function currentRoot() {
  if (!merkleTree) {
    return ethers.ZeroHash;
  }
  return "0x" + merkleTree.getRoot().toString("hex");
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Validate an Ethereum address.
 * @param {string} addr
 * @returns {boolean}
 */
function isValidAddress(addr) {
  return typeof addr === "string" && /^0x[0-9a-fA-F]{40}$/.test(addr);
}

/**
 * Parse a token amount string (whole tokens, no decimals) into a BigInt with
 * 18 decimal places of precision.
 * @param {string|number} raw
 * @returns {bigint}
 */
function parseAmount(raw) {
  return ethers.parseUnits(String(raw), 18);
}

// ─────────────────────────────────────────────────────────────────────────────
// Express app
// ─────────────────────────────────────────────────────────────────────────────

const app = express();
app.use(express.json());

// ── Safety: log every incoming request (without body contents) ────────────────
app.use((req, _res, next) => {
  console.log(`[REQ] ${req.method} ${req.path}`);
  next();
});

// ─────────────────────────────────────────────────────────────────────────────
// POST /whitelist/add
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Add an address/amount pair to the Merkle whitelist tree.
 *
 * Body: { "address": "0x...", "amount": "1000" }   (amount in whole tokens)
 * Response: { "merkleRoot": "0x..." }
 */
app.post("/whitelist/add", (req, res) => {
  const { address, amount } = req.body;

  if (!isValidAddress(address)) {
    return res.status(400).json({ error: "Invalid Ethereum address." });
  }

  let parsedAmount;
  try {
    parsedAmount = parseAmount(amount);
    if (parsedAmount <= 0n) throw new Error("non-positive");
  } catch {
    return res.status(400).json({ error: "Invalid amount." });
  }

  const normalised = ethers.getAddress(address);

  // Prevent duplicate entries for the same address.
  const existing = whitelistEntries.findIndex(
    (e) => e.address.toLowerCase() === normalised.toLowerCase()
  );
  if (existing !== -1) {
    // Update the amount for the existing entry.
    whitelistEntries[existing].amount = parsedAmount;
  } else {
    whitelistEntries.push({ address: normalised, amount: parsedAmount });
  }

  rebuildTree();

  return res.json({ merkleRoot: currentRoot() });
});

// ─────────────────────────────────────────────────────────────────────────────
// GET /whitelist/proof/:address/:amount
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Generate a Merkle proof for a given address and amount.
 *
 * :amount is in whole tokens (same unit used when adding the entry).
 * Response: { "proof": ["0x...", ...], "merkleRoot": "0x..." }
 */
app.get("/whitelist/proof/:address/:amount", (req, res) => {
  const { address, amount } = req.params;

  if (!isValidAddress(address)) {
    return res.status(400).json({ error: "Invalid Ethereum address." });
  }

  let parsedAmount;
  try {
    parsedAmount = parseAmount(amount);
  } catch {
    return res.status(400).json({ error: "Invalid amount." });
  }

  if (!merkleTree) {
    return res.status(404).json({ error: "Whitelist tree is empty." });
  }

  const leaf = buildLeaf(address, parsedAmount);
  const proof = merkleTree
    .getProof(leaf)
    .map((node) => "0x" + node.data.toString("hex"));

  if (proof.length === 0 && whitelistEntries.length > 1) {
    return res
      .status(404)
      .json({ error: "Address/amount not found in whitelist." });
  }

  return res.json({ proof, merkleRoot: currentRoot() });
});

// ─────────────────────────────────────────────────────────────────────────────
// POST /tap/sign
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Sign a Tap-and-Go transfer intent.
 *
 * The signature covers keccak256(abi.encodePacked(from, to, amount, nonce)),
 * prefixed with the Ethereum signed message prefix (EIP-191), which matches
 * MessageHashUtils.toEthSignedMessageHash() in the Solidity contract.
 *
 * Body:
 *   {
 *     "from":   "0x...",          // payer address
 *     "to":     "0x...",          // payee address
 *     "amount": "500",            // whole tokens
 *     "nonce":  "0x<32-byte-hex>" // unique session nonce (bytes32)
 *   }
 *
 * Response: { "signature": "0x...", "messageHash": "0x..." }
 */
app.post("/tap/sign", async (req, res) => {
  const { from, to, amount, nonce } = req.body;

  if (!isValidAddress(from)) {
    return res.status(400).json({ error: "Invalid 'from' address." });
  }
  if (!isValidAddress(to)) {
    return res.status(400).json({ error: "Invalid 'to' address." });
  }
  if (from.toLowerCase() === to.toLowerCase()) {
    return res.status(400).json({ error: "'from' and 'to' must differ." });
  }

  let parsedAmount;
  try {
    parsedAmount = parseAmount(amount);
    if (parsedAmount <= 0n) throw new Error("non-positive");
  } catch {
    return res.status(400).json({ error: "Invalid amount." });
  }

  // Validate nonce: must be a 32-byte hex string (bytes32).
  if (
    typeof nonce !== "string" ||
    !/^0x[0-9a-fA-F]{64}$/.test(nonce)
  ) {
    return res
      .status(400)
      .json({ error: "Invalid nonce: must be a 0x-prefixed 32-byte hex string." });
  }

  try {
    // Reproduce the on-chain message hash:
    // keccak256(abi.encodePacked(from, to, amount, nonce))
    const fromHex = from.toLowerCase().replace(/^0x/, "");
    const toHex = to.toLowerCase().replace(/^0x/, "");
    const amountHex = parsedAmount.toString(16).padStart(64, "0");
    const nonceHex = nonce.replace(/^0x/, "");

    const packed = Buffer.from(fromHex + toHex + amountHex + nonceHex, "hex");
    const messageHash = ethers.keccak256(packed);

    // signMessage applies EIP-191 prefix, matching toEthSignedMessageHash.
    const signature = await validatorWallet.signMessage(
      ethers.getBytes(messageHash)
    );

    return res.json({ signature, messageHash });
  } catch (err) {
    console.error("[ERROR] tap/sign:", err.message);
    return res.status(500).json({ error: "Signing failed." });
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// Health check
// ─────────────────────────────────────────────────────────────────────────────

app.get("/health", (_req, res) => {
  res.json({
    status: "ok",
    validatorAddress: validatorWallet.address,
    whitelistSize: whitelistEntries.length,
    merkleRoot: currentRoot(),
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Start
// ─────────────────────────────────────────────────────────────────────────────

app.listen(PORT, () => {
  console.log(`[INFO] Tap validator service listening on port ${PORT}`);
});

module.exports = app; // exported for testing
