// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/utils/cryptography/MerkleProof.sol";
import "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";

/**
 * @title FuseFlashToken
 * @notice ERC-20 token (FLSH) with Merkle-whitelist claiming, Tap-and-Go P2P
 *         transfers, and POS-checkout payments.
 */
contract FuseFlashToken is ERC20, Ownable {
    using ECDSA for bytes32;

    // ─── Constants ──────────────────────────────────────────────────────────────

    uint256 public constant TOTAL_SUPPLY = 1_000_000_000 * 10 ** 18;

    // ─── State ───────────────────────────────────────────────────────────────────

    /// @notice Merkle root used for whitelist claims.
    bytes32 public merkleRoot;

    /// @notice Address authorised to sign Tap-and-Go intents.
    address public tapValidator;

    /// @notice Tracks consumed tap nonces to prevent replay attacks.
    mapping(bytes32 => bool) public usedTapNonces;

    /// @notice Tracks which addresses have already claimed their whitelist allocation.
    mapping(address => bool) public whitelistClaimed;

    // ─── Events ──────────────────────────────────────────────────────────────────

    event MerkleRootUpdated(bytes32 indexed newRoot);
    event TapValidatorUpdated(address indexed newValidator);
    event WhitelistClaimed(address indexed claimant, uint256 amount);
    event TapAndGo(address indexed from, address indexed to, uint256 amount, bytes32 nonce);
    event PosCheckout(address indexed buyer, address indexed merchant, uint256 amount, bytes32 ref);

    // ─── Constructor ─────────────────────────────────────────────────────────────

    /**
     * @param _merkleRoot   Initial Merkle root for whitelist claims.
     * @param _tapValidator Address of the off-chain tap-validator signer.
     */
    constructor(bytes32 _merkleRoot, address _tapValidator) ERC20("Fuse Flash Token", "FLSH") {
        require(_tapValidator != address(0), "FuseFlash: zero validator");
        merkleRoot = _merkleRoot;
        tapValidator = _tapValidator;
        _mint(msg.sender, TOTAL_SUPPLY);
    }

    // ─── Owner Functions ─────────────────────────────────────────────────────────

    /**
     * @notice Update the Merkle root (e.g. after a new whitelist batch).
     * @param _merkleRoot New Merkle root.
     */
    function setMerkleRoot(bytes32 _merkleRoot) external onlyOwner {
        merkleRoot = _merkleRoot;
        emit MerkleRootUpdated(_merkleRoot);
    }

    /**
     * @notice Update the tap-validator signer address.
     * @param _tapValidator New validator address.
     */
    function setTapValidator(address _tapValidator) external onlyOwner {
        require(_tapValidator != address(0), "FuseFlash: zero validator");
        tapValidator = _tapValidator;
        emit TapValidatorUpdated(_tapValidator);
    }

    // ─── Whitelist Claim ─────────────────────────────────────────────────────────

    /**
     * @notice Claim a whitelist allocation using a Merkle proof.
     * @param amount Allocated token amount (in wei).
     * @param proof  Merkle proof path.
     */
    function claimWhitelist(uint256 amount, bytes32[] calldata proof) external {
        require(!whitelistClaimed[msg.sender], "FuseFlash: already claimed");

        bytes32 leaf = keccak256(abi.encodePacked(msg.sender, amount));
        require(MerkleProof.verify(proof, merkleRoot, leaf), "FuseFlash: invalid proof");

        whitelistClaimed[msg.sender] = true;
        _transfer(owner(), msg.sender, amount);

        emit WhitelistClaimed(msg.sender, amount);
    }

    // ─── Tap-and-Go ──────────────────────────────────────────────────────────────

    /**
     * @notice Execute a Tap-and-Go P2P transfer validated by the off-chain
     *         tap-validator.  The validator signs the intent hash; the caller
     *         (sender) submits the signature on-chain.
     *
     * @param to        Recipient address.
     * @param amount    Token amount (in wei).
     * @param nonce     Unique session nonce supplied by the tap-validator.
     * @param signature ECDSA signature produced by the tap-validator over
     *                  keccak256(abi.encodePacked(from, to, amount, nonce)).
     */
    function tapAndGo(
        address to,
        uint256 amount,
        bytes32 nonce,
        bytes calldata signature
    ) external {
        require(to != address(0), "FuseFlash: zero recipient");
        require(!usedTapNonces[nonce], "FuseFlash: nonce already used");

        bytes32 intentHash = keccak256(abi.encodePacked(msg.sender, to, amount, nonce));
        bytes32 ethSignedHash = intentHash.toEthSignedMessageHash();
        address recovered = ethSignedHash.recover(signature);
        require(recovered == tapValidator, "FuseFlash: invalid tap signature");

        usedTapNonces[nonce] = true;
        _transfer(msg.sender, to, amount);

        emit TapAndGo(msg.sender, to, amount, nonce);
    }

    // ─── POS Checkout ────────────────────────────────────────────────────────────

    /**
     * @notice Pay a merchant via QR-based POS checkout.
     * @param merchant  Merchant address to receive payment.
     * @param amount    Token amount (in wei).
     * @param ref       Unique payment reference (e.g. keccak256 of session ID).
     */
    function posCheckout(
        address merchant,
        uint256 amount,
        bytes32 ref
    ) external {
        require(merchant != address(0), "FuseFlash: zero merchant");
        require(amount > 0, "FuseFlash: zero amount");

        _transfer(msg.sender, merchant, amount);

        emit PosCheckout(msg.sender, merchant, amount, ref);
    }
}
