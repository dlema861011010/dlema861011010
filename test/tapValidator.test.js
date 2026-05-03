"use strict";

const request = require("supertest");
const { expect } = require("chai");
const { ethers } = require("ethers");
const { MerkleTree } = require("merkletreejs");
const keccak256 = require("keccak256");

// Import the app and internal state
const {
  app,
  validatorWallet,
  whitelistEntries,
  rebuildTree,
  getMerkleRoot,
  tapSignRateLimiter,
  proofRateLimiter,
} = require("../server/tapValidator");

// ─── Helpers ─────────────────────────────────────────────────────────────────

function makeLeaf(address, amount) {
  return Buffer.from(
    ethers.solidityPackedKeccak256(["address", "uint256"], [address, amount]).slice(2),
    "hex"
  );
}

// ─── Tests ───────────────────────────────────────────────────────────────────

describe("tapValidator server", function () {
  // Reset whitelist state and rate limiters before each test
  beforeEach(function () {
    whitelistEntries.clear();
    rebuildTree();
    tapSignRateLimiter.reset();
    proofRateLimiter.reset();
  });

  // ─── /health ───────────────────────────────────────────────────────────────

  describe("GET /health", function () {
    it("should return status ok and validator address", async function () {
      const res = await request(app).get("/health").expect(200);
      expect(res.body.status).to.equal("ok");
      expect(res.body.validator).to.equal(validatorWallet.address);
    });
  });

  // ─── POST /whitelist/add ───────────────────────────────────────────────────

  describe("POST /whitelist/add", function () {
    it("should add an entry and return a merkle root", async function () {
      const address = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8";
      const amount = "1000000000000000000000"; // 1000 tokens in wei

      const res = await request(app)
        .post("/whitelist/add")
        .send({ address, amount })
        .expect(200);

      expect(res.body.merkleRoot).to.be.a("string");
      expect(res.body.merkleRoot).to.match(/^0x[0-9a-f]{64}$/);
    });

    it("should return 400 if address is missing", async function () {
      const res = await request(app)
        .post("/whitelist/add")
        .send({ amount: "1000" })
        .expect(400);
      expect(res.body.error).to.exist;
    });

    it("should return 400 if amount is missing", async function () {
      const res = await request(app)
        .post("/whitelist/add")
        .send({ address: "0x70997970C51812dc3A010C7d01b50e0d17dc79C8" })
        .expect(400);
      expect(res.body.error).to.exist;
    });

    it("should return 400 for an invalid address", async function () {
      const res = await request(app)
        .post("/whitelist/add")
        .send({ address: "not-an-address", amount: "1000" })
        .expect(400);
      expect(res.body.error).to.include("invalid address");
    });

    it("should return 400 for a zero amount", async function () {
      const res = await request(app)
        .post("/whitelist/add")
        .send({ address: "0x70997970C51812dc3A010C7d01b50e0d17dc79C8", amount: "0" })
        .expect(400);
      expect(res.body.error).to.exist;
    });

    it("should overwrite an existing entry for the same address", async function () {
      const address = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8";

      await request(app).post("/whitelist/add").send({ address, amount: "1000" }).expect(200);
      const res = await request(app)
        .post("/whitelist/add")
        .send({ address, amount: "2000" })
        .expect(200);

      expect(res.body.merkleRoot).to.be.a("string");
      // The entry should be updated (only one leaf in the tree)
    });

    it("should update the merkle root as more entries are added", async function () {
      const addr1 = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8";
      const addr2 = "0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC";

      const res1 = await request(app)
        .post("/whitelist/add")
        .send({ address: addr1, amount: "1000" })
        .expect(200);

      const res2 = await request(app)
        .post("/whitelist/add")
        .send({ address: addr2, amount: "2000" })
        .expect(200);

      expect(res1.body.merkleRoot).to.not.equal(res2.body.merkleRoot);
    });
  });

  // ─── GET /whitelist/proof/:address/:amount ─────────────────────────────────

  describe("GET /whitelist/proof/:address/:amount", function () {
    const address = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8";
    const amount = "1000000000000000000000";

    beforeEach(async function () {
      await request(app).post("/whitelist/add").send({ address, amount });
    });

    it("should return a valid proof for a whitelisted entry", async function () {
      const res = await request(app)
        .get(`/whitelist/proof/${address}/${amount}`)
        .expect(200);

      expect(res.body.proof).to.be.an("array");
      expect(res.body.merkleRoot).to.be.a("string");
    });

    it("should return a proof that verifies against the returned root", async function () {
      const res = await request(app)
        .get(`/whitelist/proof/${address}/${amount}`)
        .expect(200);

      const { proof, merkleRoot } = res.body;

      // Re-verify using MerkleTree
      const leaf = makeLeaf(ethers.getAddress(address), amount);
      const tree = new MerkleTree([leaf], keccak256, { sortPairs: true });
      const rootHex = "0x" + tree.getRoot().toString("hex");

      // With a single leaf, proof is empty and root equals the leaf hash
      expect(rootHex).to.equal(merkleRoot);
    });

    it("should return 400 for an invalid address param", async function () {
      const res = await request(app)
        .get(`/whitelist/proof/not-an-address/${amount}`)
        .expect(400);
      expect(res.body.error).to.include("invalid address");
    });

    it("should return 404 when whitelist is empty", async function () {
      whitelistEntries.clear();
      rebuildTree();

      const res = await request(app)
        .get(`/whitelist/proof/${address}/${amount}`)
        .expect(404);
      expect(res.body.error).to.equal("whitelist is empty");
    });

    it("should return 404 for an address not in the whitelist", async function () {
      const unknownAddress = "0x90F79bf6EB2c4f870365E785982E1f101E93b906";
      const res = await request(app)
        .get(`/whitelist/proof/${unknownAddress}/${amount}`)
        .expect(404);
      expect(res.body.error).to.include("not in whitelist");
    });

    it("should return 404 for a correct address but wrong amount", async function () {
      const wrongAmount = "9999";
      const res = await request(app)
        .get(`/whitelist/proof/${address}/${wrongAmount}`)
        .expect(404);
      expect(res.body.error).to.include("not in whitelist");
    });
  });

  // ─── POST /tap/sign ────────────────────────────────────────────────────────

  describe("POST /tap/sign", function () {
    const from = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8";
    const to = "0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC";
    const amount = "1000000000000000000"; // 1 token in wei
    const nonce = "0x" + "ab".repeat(32); // 32-byte nonce

    it("should return a valid signature and intentHash", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ from, to, amount, nonce })
        .expect(200);

      expect(res.body.signature).to.be.a("string");
      expect(res.body.intentHash).to.match(/^0x[0-9a-f]{64}$/);
    });

    it("should produce a signature that recovers to the validator address", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ from, to, amount, nonce })
        .expect(200);

      const { signature, intentHash } = res.body;

      // Recover signer
      const recovered = ethers.verifyMessage(ethers.getBytes(intentHash), signature);
      expect(recovered).to.equal(validatorWallet.address);
    });

    it("should produce a deterministic signature for the same inputs", async function () {
      const res1 = await request(app).post("/tap/sign").send({ from, to, amount, nonce });
      const res2 = await request(app).post("/tap/sign").send({ from, to, amount, nonce });

      // ECDSA with deterministic k (ethers.js default) should give same signature
      expect(res1.body.signature).to.equal(res2.body.signature);
    });

    it("should return 400 if from is missing", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ to, amount, nonce })
        .expect(400);
      expect(res.body.error).to.exist;
    });

    it("should return 400 if to is missing", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ from, amount, nonce })
        .expect(400);
      expect(res.body.error).to.exist;
    });

    it("should return 400 if amount is missing", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ from, to, nonce })
        .expect(400);
      expect(res.body.error).to.exist;
    });

    it("should return 400 if nonce is missing", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ from, to, amount })
        .expect(400);
      expect(res.body.error).to.exist;
    });

    it("should return 400 for an invalid from address", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ from: "invalid", to, amount, nonce })
        .expect(400);
      expect(res.body.error).to.include("invalid address");
    });

    it("should return 400 for an invalid to address", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ from, to: "invalid", amount, nonce })
        .expect(400);
      expect(res.body.error).to.include("invalid address");
    });

    it("should return 400 for a nonce that is not 32 bytes hex", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ from, to, amount, nonce: "0xshort" })
        .expect(400);
      expect(res.body.error).to.include("nonce must be a 32-byte hex string");
    });

    it("should return 400 for a zero amount", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ from, to, amount: "0", nonce })
        .expect(400);
      expect(res.body.error).to.exist;
    });

    it("should return 400 for a negative amount", async function () {
      const res = await request(app)
        .post("/tap/sign")
        .send({ from, to, amount: "-1", nonce })
        .expect(400);
      expect(res.body.error).to.exist;
    });

    it("should produce different signatures for different nonces", async function () {
      const nonce2 = "0x" + "cd".repeat(32);
      const res1 = await request(app).post("/tap/sign").send({ from, to, amount, nonce });
      const res2 = await request(app).post("/tap/sign").send({ from, to, amount, nonce: nonce2 });

      expect(res1.body.intentHash).to.not.equal(res2.body.intentHash);
      expect(res1.body.signature).to.not.equal(res2.body.signature);
    });

    it("should produce different intent hashes for different amounts", async function () {
      const amount2 = "2000000000000000000";
      const res1 = await request(app).post("/tap/sign").send({ from, to, amount, nonce });
      const res2 = await request(app).post("/tap/sign").send({ from, to, amount: amount2, nonce });

      expect(res1.body.intentHash).to.not.equal(res2.body.intentHash);
    });

    it("should return 429 after exceeding the rate limit", async function () {
      // Reset first to ensure clean state, then exhaust the limit (10 req/min)
      tapSignRateLimiter.reset();
      const validBody = { from, to, amount, nonce };
      const nonces = Array.from({ length: 10 }, (_, i) => "0x" + i.toString(16).padStart(64, "0"));

      for (const n of nonces) {
        await request(app).post("/tap/sign").send({ ...validBody, nonce: n }).expect(200);
      }

      // 11th request should be rate-limited
      const res = await request(app).post("/tap/sign").send(validBody).expect(429);
      expect(res.body.error).to.include("Too many requests");
    });
  });
});
