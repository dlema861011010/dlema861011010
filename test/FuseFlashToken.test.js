"use strict";

const { expect } = require("chai");
const { ethers } = require("hardhat");
const { MerkleTree } = require("merkletreejs");
const keccak256 = require("keccak256");

// ─── Helpers ─────────────────────────────────────────────────────────────────

/**
 * Build a Merkle tree from an array of { address, amount } entries.
 * Leaves are keccak256(abi.encodePacked(address, amount)).
 */
function buildWhitelistTree(entries) {
  const leaves = entries.map(({ address, amount }) =>
    ethers.solidityPackedKeccak256(["address", "uint256"], [address, amount])
  );
  return new MerkleTree(leaves, keccak256, { sortPairs: true });
}

function getLeaf(address, amount) {
  return Buffer.from(
    ethers.solidityPackedKeccak256(["address", "uint256"], [address, amount]).slice(2),
    "hex"
  );
}

// ─── Tests ───────────────────────────────────────────────────────────────────

describe("FuseFlashToken", function () {
  let token;
  let owner;
  let tapValidator;
  let alice;
  let bob;
  let charlie;

  const TOTAL_SUPPLY = ethers.parseEther("1000000000"); // 1 Billion

  beforeEach(async function () {
    [owner, tapValidator, alice, bob, charlie] = await ethers.getSigners();

    const FuseFlashToken = await ethers.getContractFactory("FuseFlashToken");
    token = await FuseFlashToken.deploy(ethers.ZeroHash, tapValidator.address);
    await token.waitForDeployment();
  });

  // ─── Deployment ────────────────────────────────────────────────────────────

  describe("Deployment", function () {
    it("should have the correct name", async function () {
      expect(await token.name()).to.equal("Fuse Flash Token");
    });

    it("should have the correct symbol", async function () {
      expect(await token.symbol()).to.equal("FLSH");
    });

    it("should mint the full supply to the owner", async function () {
      expect(await token.totalSupply()).to.equal(TOTAL_SUPPLY);
      expect(await token.balanceOf(owner.address)).to.equal(TOTAL_SUPPLY);
    });

    it("should set TOTAL_SUPPLY constant to 1 Billion tokens", async function () {
      expect(await token.TOTAL_SUPPLY()).to.equal(TOTAL_SUPPLY);
    });

    it("should set the tapValidator address", async function () {
      expect(await token.tapValidator()).to.equal(tapValidator.address);
    });

    it("should set the initial merkleRoot to the provided value", async function () {
      const root = ethers.keccak256(ethers.toUtf8Bytes("test-root"));
      const FuseFlashToken = await ethers.getContractFactory("FuseFlashToken");
      const t = await FuseFlashToken.deploy(root, tapValidator.address);
      expect(await t.merkleRoot()).to.equal(root);
    });

    it("should revert if tapValidator is zero address", async function () {
      const FuseFlashToken = await ethers.getContractFactory("FuseFlashToken");
      await expect(
        FuseFlashToken.deploy(ethers.ZeroHash, ethers.ZeroAddress)
      ).to.be.revertedWith("FuseFlash: zero validator");
    });

    it("should have the owner set correctly", async function () {
      expect(await token.owner()).to.equal(owner.address);
    });
  });

  // ─── Owner Functions ───────────────────────────────────────────────────────

  describe("setMerkleRoot", function () {
    it("should allow the owner to update the merkle root", async function () {
      const newRoot = ethers.keccak256(ethers.toUtf8Bytes("new-root"));
      await token.connect(owner).setMerkleRoot(newRoot);
      expect(await token.merkleRoot()).to.equal(newRoot);
    });

    it("should emit MerkleRootUpdated event", async function () {
      const newRoot = ethers.keccak256(ethers.toUtf8Bytes("new-root"));
      await expect(token.connect(owner).setMerkleRoot(newRoot))
        .to.emit(token, "MerkleRootUpdated")
        .withArgs(newRoot);
    });

    it("should revert if called by non-owner", async function () {
      const newRoot = ethers.keccak256(ethers.toUtf8Bytes("new-root"));
      await expect(token.connect(alice).setMerkleRoot(newRoot)).to.be.revertedWith(
        "Ownable: caller is not the owner"
      );
    });
  });

  describe("setTapValidator", function () {
    it("should allow the owner to update the tap validator", async function () {
      await token.connect(owner).setTapValidator(alice.address);
      expect(await token.tapValidator()).to.equal(alice.address);
    });

    it("should emit TapValidatorUpdated event", async function () {
      await expect(token.connect(owner).setTapValidator(alice.address))
        .to.emit(token, "TapValidatorUpdated")
        .withArgs(alice.address);
    });

    it("should revert if new validator is zero address", async function () {
      await expect(
        token.connect(owner).setTapValidator(ethers.ZeroAddress)
      ).to.be.revertedWith("FuseFlash: zero validator");
    });

    it("should revert if called by non-owner", async function () {
      await expect(
        token.connect(alice).setTapValidator(bob.address)
      ).to.be.revertedWith("Ownable: caller is not the owner");
    });
  });

  // ─── Whitelist Claim ───────────────────────────────────────────────────────

  describe("claimWhitelist", function () {
    let tree;
    let aliceAmount;
    let bobAmount;

    beforeEach(async function () {
      aliceAmount = ethers.parseEther("1000");
      bobAmount = ethers.parseEther("2000");

      const entries = [
        { address: alice.address, amount: aliceAmount },
        { address: bob.address, amount: bobAmount },
      ];
      tree = buildWhitelistTree(entries);
      const root = "0x" + tree.getRoot().toString("hex");
      await token.connect(owner).setMerkleRoot(root);

      // Fund the owner account with enough tokens (already minted to owner)
    });

    it("should allow a whitelisted address to claim tokens", async function () {
      const proof = tree.getHexProof(getLeaf(alice.address, aliceAmount));
      const ownerBalanceBefore = await token.balanceOf(owner.address);

      await token.connect(alice).claimWhitelist(aliceAmount, proof);

      expect(await token.balanceOf(alice.address)).to.equal(aliceAmount);
      expect(await token.balanceOf(owner.address)).to.equal(ownerBalanceBefore - aliceAmount);
    });

    it("should emit WhitelistClaimed event", async function () {
      const proof = tree.getHexProof(getLeaf(alice.address, aliceAmount));
      await expect(token.connect(alice).claimWhitelist(aliceAmount, proof))
        .to.emit(token, "WhitelistClaimed")
        .withArgs(alice.address, aliceAmount);
    });

    it("should mark the address as claimed after successful claim", async function () {
      const proof = tree.getHexProof(getLeaf(alice.address, aliceAmount));
      await token.connect(alice).claimWhitelist(aliceAmount, proof);
      expect(await token.whitelistClaimed(alice.address)).to.be.true;
    });

    it("should revert on a second claim attempt (double-spend)", async function () {
      const proof = tree.getHexProof(getLeaf(alice.address, aliceAmount));
      await token.connect(alice).claimWhitelist(aliceAmount, proof);
      await expect(
        token.connect(alice).claimWhitelist(aliceAmount, proof)
      ).to.be.revertedWith("FuseFlash: already claimed");
    });

    it("should revert with an invalid proof", async function () {
      const wrongProof = tree.getHexProof(getLeaf(bob.address, bobAmount));
      await expect(
        token.connect(alice).claimWhitelist(aliceAmount, wrongProof)
      ).to.be.revertedWith("FuseFlash: invalid proof");
    });

    it("should revert with a wrong amount for a valid address", async function () {
      const wrongAmount = ethers.parseEther("999");
      const proof = tree.getHexProof(getLeaf(alice.address, aliceAmount));
      await expect(
        token.connect(alice).claimWhitelist(wrongAmount, proof)
      ).to.be.revertedWith("FuseFlash: invalid proof");
    });

    it("should allow multiple different addresses to claim", async function () {
      const aliceProof = tree.getHexProof(getLeaf(alice.address, aliceAmount));
      const bobProof = tree.getHexProof(getLeaf(bob.address, bobAmount));

      await token.connect(alice).claimWhitelist(aliceAmount, aliceProof);
      await token.connect(bob).claimWhitelist(bobAmount, bobProof);

      expect(await token.balanceOf(alice.address)).to.equal(aliceAmount);
      expect(await token.balanceOf(bob.address)).to.equal(bobAmount);
    });

    it("should revert for an address not in the whitelist", async function () {
      const proof = tree.getHexProof(getLeaf(charlie.address, aliceAmount));
      await expect(
        token.connect(charlie).claimWhitelist(aliceAmount, proof)
      ).to.be.revertedWith("FuseFlash: invalid proof");
    });
  });

  // ─── Tap-and-Go ────────────────────────────────────────────────────────────

  describe("tapAndGo", function () {
    const TRANSFER_AMOUNT = ethers.parseEther("100");

    /**
     * Create a tap intent signature using the tapValidator signer.
     */
    async function signTapIntent(from, to, amount, nonce) {
      const intentHash = ethers.solidityPackedKeccak256(
        ["address", "address", "uint256", "bytes32"],
        [from, to, amount, nonce]
      );
      return tapValidator.signMessage(ethers.getBytes(intentHash));
    }

    beforeEach(async function () {
      // Give alice some tokens to send
      await token.connect(owner).transfer(alice.address, ethers.parseEther("1000"));
    });

    it("should execute a valid tap transfer", async function () {
      const nonce = ethers.id("tap-session-1");
      const sig = await signTapIntent(alice.address, bob.address, TRANSFER_AMOUNT, nonce);

      const aliceBefore = await token.balanceOf(alice.address);
      const bobBefore = await token.balanceOf(bob.address);

      await token.connect(alice).tapAndGo(bob.address, TRANSFER_AMOUNT, nonce, sig);

      expect(await token.balanceOf(alice.address)).to.equal(aliceBefore - TRANSFER_AMOUNT);
      expect(await token.balanceOf(bob.address)).to.equal(bobBefore + TRANSFER_AMOUNT);
    });

    it("should emit TapAndGo event", async function () {
      const nonce = ethers.id("tap-session-2");
      const sig = await signTapIntent(alice.address, bob.address, TRANSFER_AMOUNT, nonce);

      await expect(token.connect(alice).tapAndGo(bob.address, TRANSFER_AMOUNT, nonce, sig))
        .to.emit(token, "TapAndGo")
        .withArgs(alice.address, bob.address, TRANSFER_AMOUNT, nonce);
    });

    it("should mark the nonce as used after a successful tap", async function () {
      const nonce = ethers.id("tap-session-3");
      const sig = await signTapIntent(alice.address, bob.address, TRANSFER_AMOUNT, nonce);
      await token.connect(alice).tapAndGo(bob.address, TRANSFER_AMOUNT, nonce, sig);
      expect(await token.usedTapNonces(nonce)).to.be.true;
    });

    it("should revert on nonce replay", async function () {
      const nonce = ethers.id("tap-session-4");
      const sig = await signTapIntent(alice.address, bob.address, TRANSFER_AMOUNT, nonce);
      await token.connect(alice).tapAndGo(bob.address, TRANSFER_AMOUNT, nonce, sig);

      await expect(
        token.connect(alice).tapAndGo(bob.address, TRANSFER_AMOUNT, nonce, sig)
      ).to.be.revertedWith("FuseFlash: nonce already used");
    });

    it("should revert with an invalid (wrong signer) signature", async function () {
      const nonce = ethers.id("tap-session-5");
      // Sign with alice, not the tapValidator
      const sig = await alice.signMessage(
        ethers.getBytes(
          ethers.solidityPackedKeccak256(
            ["address", "address", "uint256", "bytes32"],
            [alice.address, bob.address, TRANSFER_AMOUNT, nonce]
          )
        )
      );
      await expect(
        token.connect(alice).tapAndGo(bob.address, TRANSFER_AMOUNT, nonce, sig)
      ).to.be.revertedWith("FuseFlash: invalid tap signature");
    });

    it("should revert if the recipient is zero address", async function () {
      const nonce = ethers.id("tap-session-6");
      const sig = await signTapIntent(alice.address, ethers.ZeroAddress, TRANSFER_AMOUNT, nonce);
      await expect(
        token.connect(alice).tapAndGo(ethers.ZeroAddress, TRANSFER_AMOUNT, nonce, sig)
      ).to.be.revertedWith("FuseFlash: zero recipient");
    });

    it("should revert if signature was created for a different amount", async function () {
      const nonce = ethers.id("tap-session-7");
      const sig = await signTapIntent(alice.address, bob.address, TRANSFER_AMOUNT, nonce);
      const differentAmount = ethers.parseEther("999");
      await expect(
        token.connect(alice).tapAndGo(bob.address, differentAmount, nonce, sig)
      ).to.be.revertedWith("FuseFlash: invalid tap signature");
    });

    it("should revert if signature was created for a different recipient", async function () {
      const nonce = ethers.id("tap-session-8");
      const sig = await signTapIntent(alice.address, charlie.address, TRANSFER_AMOUNT, nonce);
      await expect(
        token.connect(alice).tapAndGo(bob.address, TRANSFER_AMOUNT, nonce, sig)
      ).to.be.revertedWith("FuseFlash: invalid tap signature");
    });

    it("should allow two taps with different nonces", async function () {
      const nonce1 = ethers.id("tap-session-9a");
      const nonce2 = ethers.id("tap-session-9b");
      const sig1 = await signTapIntent(alice.address, bob.address, TRANSFER_AMOUNT, nonce1);
      const sig2 = await signTapIntent(alice.address, bob.address, TRANSFER_AMOUNT, nonce2);

      await token.connect(alice).tapAndGo(bob.address, TRANSFER_AMOUNT, nonce1, sig1);
      await token.connect(alice).tapAndGo(bob.address, TRANSFER_AMOUNT, nonce2, sig2);

      expect(await token.balanceOf(bob.address)).to.equal(TRANSFER_AMOUNT * 2n);
    });
  });

  // ─── POS Checkout ──────────────────────────────────────────────────────────

  describe("posCheckout", function () {
    const PRICE = ethers.parseEther("50");

    beforeEach(async function () {
      await token.connect(owner).transfer(alice.address, ethers.parseEther("500"));
    });

    it("should transfer tokens from buyer to merchant", async function () {
      const ref = ethers.id("pos-ref-1");
      const aliceBefore = await token.balanceOf(alice.address);
      const bobBefore = await token.balanceOf(bob.address);

      await token.connect(alice).posCheckout(bob.address, PRICE, ref);

      expect(await token.balanceOf(alice.address)).to.equal(aliceBefore - PRICE);
      expect(await token.balanceOf(bob.address)).to.equal(bobBefore + PRICE);
    });

    it("should emit PosCheckout event", async function () {
      const ref = ethers.id("pos-ref-2");
      await expect(token.connect(alice).posCheckout(bob.address, PRICE, ref))
        .to.emit(token, "PosCheckout")
        .withArgs(alice.address, bob.address, PRICE, ref);
    });

    it("should revert if merchant is zero address", async function () {
      const ref = ethers.id("pos-ref-3");
      await expect(
        token.connect(alice).posCheckout(ethers.ZeroAddress, PRICE, ref)
      ).to.be.revertedWith("FuseFlash: zero merchant");
    });

    it("should revert if amount is zero", async function () {
      const ref = ethers.id("pos-ref-4");
      await expect(
        token.connect(alice).posCheckout(bob.address, 0, ref)
      ).to.be.revertedWith("FuseFlash: zero amount");
    });

    it("should revert if buyer has insufficient balance", async function () {
      const ref = ethers.id("pos-ref-5");
      const hugeAmount = ethers.parseEther("1000000");
      await expect(
        token.connect(alice).posCheckout(bob.address, hugeAmount, ref)
      ).to.be.revertedWith("ERC20: transfer amount exceeds balance");
    });

    it("should support multiple checkouts from the same buyer", async function () {
      const ref1 = ethers.id("pos-ref-6a");
      const ref2 = ethers.id("pos-ref-6b");

      await token.connect(alice).posCheckout(bob.address, PRICE, ref1);
      await token.connect(alice).posCheckout(charlie.address, PRICE, ref2);

      expect(await token.balanceOf(bob.address)).to.equal(PRICE);
      expect(await token.balanceOf(charlie.address)).to.equal(PRICE);
    });
  });

  // ─── ERC-20 Standard Behaviour ─────────────────────────────────────────────

  describe("ERC-20 standard behaviour", function () {
    it("should support transfer", async function () {
      const amount = ethers.parseEther("100");
      await token.connect(owner).transfer(alice.address, amount);
      expect(await token.balanceOf(alice.address)).to.equal(amount);
    });

    it("should support approve and transferFrom", async function () {
      const amount = ethers.parseEther("100");
      await token.connect(owner).approve(alice.address, amount);
      await token.connect(alice).transferFrom(owner.address, bob.address, amount);
      expect(await token.balanceOf(bob.address)).to.equal(amount);
    });

    it("should return correct decimals (18)", async function () {
      expect(await token.decimals()).to.equal(18);
    });
  });
});
