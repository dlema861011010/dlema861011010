// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/utils/cryptography/MerkleProof.sol";
import "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import "@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol";

/**
 * @title FuseFlashToken
 * @notice ERC-20 token integrating Tap-and-Go P2P payments and Merkle-based
 *         whitelist claims for the Fuse Flash ecosystem.
 *
 * Architecture
 * ============
 * 1. Merkle Whitelist  — users prove allocation eligibility off-chain and call
 *    claimWhitelist() to receive their tokens.
 * 2. Tap-and-Go        — the tapValidator backend signs transfer intents;
 *    on-chain ECDSA verification forwards tokens from the payer to the payee.
 * 3. Replay Protection — usedTapNonces tracks consumed tap sessions.
 * 4. POS Checkout      — posCheckout() processes QR-based POS payments in one
 *    on-chain call, emitting a receipt event for off-chain indexers.
 */
contract FuseFlashToken is ERC20, Ownable {
    using ECDSA for bytes32;

    // ──────────────────────────────────────────────────────────────────────────
    // Constants & immutables
    // ──────────────────────────────────────────────────────────────────────────

    /// @notice Total fixed supply: 1,000,000,000 FLSH (1 Billion).
    uint256 public constant TOTAL_SUPPLY = 1_000_000_000 * 10 ** 18;

    // ──────────────────────────────────────────────────────────────────────────
    // State
    // ──────────────────────────────────────────────────────────────────────────

    /// @notice Merkle root for the whitelist allocation tree.
    bytes32 public merkleRoot;

    /// @notice Address of the trusted off-chain tap validator.
    address public tapValidator;

    /// @notice Tracks addresses that have already claimed their whitelist
    ///         allocation (prevents double-claims).
    mapping(address => bool) public whitelistClaimed;

    /// @notice Tracks consumed tap nonces for replay protection.
    ///         nonce → consumed
    mapping(bytes32 => bool) public usedTapNonces;

    // ──────────────────────────────────────────────────────────────────────────
    // Events
    // ──────────────────────────────────────────────────────────────────────────

    event MerkleRootUpdated(bytes32 indexed newRoot);
    event TapValidatorUpdated(address indexed newValidator);
    event WhitelistClaimed(address indexed claimant, uint256 amount);
    event TapAndGo(
        address indexed from,
        address indexed to,
        uint256 amount,
        bytes32 indexed nonce
    );
    event PosCheckout(
        address indexed payer,
        address indexed merchant,
        uint256 amount,
        bytes32 indexed sessionId
    );

    // ──────────────────────────────────────────────────────────────────────────
    // Constructor
    // ──────────────────────────────────────────────────────────────────────────

    /**
     * @param initialOwner  Address that will own the contract.
     * @param _tapValidator Address of the off-chain tap validator service.
     * @param _merkleRoot   Initial Merkle root for whitelist allocations.
     */
    constructor(
        address initialOwner,
        address _tapValidator,
        bytes32 _merkleRoot
    ) ERC20("Fuse Flash Token", "FLSH") Ownable(initialOwner) {
        require(_tapValidator != address(0), "FuseFlash: zero validator");
        tapValidator = _tapValidator;
        merkleRoot = _merkleRoot;
        // Mint entire supply to the owner for distribution.
        _mint(initialOwner, TOTAL_SUPPLY);
    }

    // ──────────────────────────────────────────────────────────────────────────
    // Owner-only configuration
    // ──────────────────────────────────────────────────────────────────────────

    /**
     * @notice Update the Merkle root (e.g. after adding new whitelist members).
     * @param _merkleRoot New Merkle root published by the validator service.
     */
    function setMerkleRoot(bytes32 _merkleRoot) external onlyOwner {
        merkleRoot = _merkleRoot;
        emit MerkleRootUpdated(_merkleRoot);
    }

    /**
     * @notice Replace the tap validator address.
     * @param _tapValidator New validator address.
     */
    function setTapValidator(address _tapValidator) external onlyOwner {
        require(_tapValidator != address(0), "FuseFlash: zero validator");
        tapValidator = _tapValidator;
        emit TapValidatorUpdated(_tapValidator);
    }

    // ──────────────────────────────────────────────────────────────────────────
    // Merkle whitelist claim
    // ──────────────────────────────────────────────────────────────────────────

    /**
     * @notice Claim a whitelist allocation by supplying a valid Merkle proof.
     *
     * The leaf is keccak256(abi.encodePacked(claimant, amount)), which matches
     * the leaf format produced by the tapValidator backend.
     *
     * @param amount Amount of FLSH (in 18-decimal base units) to claim.
     * @param proof  Merkle proof path.
     */
    function claimWhitelist(
        uint256 amount,
        bytes32[] calldata proof
    ) external {
        address claimant = msg.sender;
        require(!whitelistClaimed[claimant], "FuseFlash: already claimed");

        bytes32 leaf = keccak256(abi.encodePacked(claimant, amount));
        require(
            MerkleProof.verify(proof, merkleRoot, leaf),
            "FuseFlash: invalid proof"
        );

        whitelistClaimed[claimant] = true;
        _transfer(owner(), claimant, amount);
        emit WhitelistClaimed(claimant, amount);
    }

    // ──────────────────────────────────────────────────────────────────────────
    // Tap-and-Go P2P transfer
    // ──────────────────────────────────────────────────────────────────────────

    /**
     * @notice Execute a Tap-and-Go P2P transfer authenticated by the validator.
     *
     * The validator signs keccak256(abi.encodePacked(from, to, amount, nonce))
     * using its private key.  On-chain ECDSA recovery confirms the intent was
     * authorised by the trusted validator service, then the token transfer is
     * executed.
     *
     * Replay protection: each nonce may only be used once.
     *
     * @param from      Payer address (must have approved this contract or be
     *                  msg.sender).
     * @param to        Payee address.
     * @param amount    Amount of FLSH in base units.
     * @param nonce     Unique tap session identifier supplied by the validator.
     * @param signature ECDSA signature from the tapValidator.
     */
    function tapAndGo(
        address from,
        address to,
        uint256 amount,
        bytes32 nonce,
        bytes calldata signature
    ) external {
        require(!usedTapNonces[nonce], "FuseFlash: nonce already used");
        require(to != address(0), "FuseFlash: zero recipient");
        require(amount > 0, "FuseFlash: zero amount");

        // Reconstruct the signed message hash.
        bytes32 messageHash = keccak256(
            abi.encodePacked(from, to, amount, nonce)
        );
        bytes32 ethHash = MessageHashUtils.toEthSignedMessageHash(messageHash);

        // Verify the signature was produced by the trusted tapValidator.
        address recovered = ethHash.recover(signature);
        require(recovered == tapValidator, "FuseFlash: invalid tap signature");

        // Mark nonce as consumed before transferring (checks-effects-interactions).
        usedTapNonces[nonce] = true;

        // Execute the transfer.  The caller (msg.sender) may be the payer
        // themselves (NFC tap) or a relayer acting on their behalf.
        if (from == msg.sender) {
            _transfer(from, to, amount);
        } else {
            // Relayer path: requires prior approval from `from`.
            transferFrom(from, to, amount);
        }

        emit TapAndGo(from, to, amount, nonce);
    }

    // ──────────────────────────────────────────────────────────────────────────
    // POS Checkout
    // ──────────────────────────────────────────────────────────────────────────

    /**
     * @notice Process a QR-based POS payment.
     *
     * The payer calls this function (or a relayer calls it on their behalf)
     * after scanning the merchant's QR code.  sessionId provides idempotency —
     * the same checkout session cannot be processed twice.
     *
     * @param merchant   Merchant's receiving address.
     * @param amount     Amount of FLSH in base units.
     * @param sessionId  Unique POS session identifier (e.g. keccak256 of POS
     *                   terminal ID + timestamp).
     */
    function posCheckout(
        address merchant,
        uint256 amount,
        bytes32 sessionId
    ) external {
        require(!usedTapNonces[sessionId], "FuseFlash: session already used");
        require(merchant != address(0), "FuseFlash: zero merchant");
        require(amount > 0, "FuseFlash: zero amount");

        // Mark session as consumed before the transfer.
        usedTapNonces[sessionId] = true;

        _transfer(msg.sender, merchant, amount);
        emit PosCheckout(msg.sender, merchant, amount, sessionId);
    }
}
