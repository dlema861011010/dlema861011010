// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./interfaces/IERC20.sol";
import "./FuseToken.sol";

/**
 * @title FuseBridge
 * @notice Lock-and-mint bridge between the Fuse Network and Ethereum.
 *
 * Flow – Fuse → Ethereum (deposit):
 *   1. A relayer observes a native FUSE lock on the Fuse Network.
 *   2. The relayer calls `mint()` on this contract, which in turn calls
 *      `FuseToken.mint()` to issue wrapped FUSE (wFUSE) to the recipient.
 *
 * Flow – Ethereum → Fuse (withdrawal):
 *   1. A user calls `burn()`, sending wFUSE back to the bridge.
 *   2. The bridge calls `FuseToken.burn()` and emits `TokensBurned`.
 *   3. A relayer observes `TokensBurned` and unlocks native FUSE on
 *      the Fuse Network.
 *
 * Security model:
 *   - Only addresses in the `relayers` set may call `mint()`.
 *   - The contract owner may add / remove relayers and pause the bridge.
 */
contract FuseBridge {
    // -------------------------------------------------------------------------
    // State
    // -------------------------------------------------------------------------
    address public owner;
    FuseToken public fuseToken;
    bool public paused;

    mapping(address => bool) public relayers;

    // Nonce tracking to prevent replay of the same relayer message.
    mapping(bytes32 => bool) public processedTxHashes;

    // -------------------------------------------------------------------------
    // Events
    // -------------------------------------------------------------------------
    event TokensMinted(
        address indexed recipient,
        uint256 amount,
        bytes32 indexed fuseNetworkTxHash
    );
    event TokensBurned(
        address indexed sender,
        uint256 amount,
        string  fuseNetworkRecipient
    );
    event RelayerAdded(address indexed relayer);
    event RelayerRemoved(address indexed relayer);
    event BridgePaused(bool paused);
    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);

    // -------------------------------------------------------------------------
    // Modifiers
    // -------------------------------------------------------------------------
    modifier onlyOwner() {
        require(msg.sender == owner, "FuseBridge: caller is not owner");
        _;
    }

    modifier onlyRelayer() {
        require(relayers[msg.sender], "FuseBridge: caller is not a relayer");
        _;
    }

    modifier whenNotPaused() {
        require(!paused, "FuseBridge: bridge is paused");
        _;
    }

    // -------------------------------------------------------------------------
    // Constructor
    // -------------------------------------------------------------------------
    /**
     * @param _fuseToken Address of the deployed FuseToken contract.
     */
    constructor(address _fuseToken) {
        require(_fuseToken != address(0), "FuseBridge: zero token address");
        owner     = msg.sender;
        fuseToken = FuseToken(_fuseToken);
        relayers[msg.sender] = true;
        emit RelayerAdded(msg.sender);
    }

    // -------------------------------------------------------------------------
    // Administration
    // -------------------------------------------------------------------------

    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "FuseBridge: zero address");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    function addRelayer(address relayer) external onlyOwner {
        require(relayer != address(0), "FuseBridge: zero address");
        relayers[relayer] = true;
        emit RelayerAdded(relayer);
    }

    function removeRelayer(address relayer) external onlyOwner {
        relayers[relayer] = false;
        emit RelayerRemoved(relayer);
    }

    function setPaused(bool _paused) external onlyOwner {
        paused = _paused;
        emit BridgePaused(_paused);
    }

    // -------------------------------------------------------------------------
    // Bridge operations
    // -------------------------------------------------------------------------

    /**
     * @notice Mint wrapped FUSE on Ethereum after FUSE is locked on Fuse Network.
     * @dev Called by an authorised relayer. Each Fuse Network tx may only be
     *      processed once (replay protection via `fuseNetworkTxHash`).
     * @param recipient         Ethereum address to receive the tokens.
     * @param amount            Amount of FUSE (18 decimals).
     * @param fuseNetworkTxHash Hash of the lock transaction on Fuse Network.
     */
    function mint(
        address recipient,
        uint256 amount,
        bytes32 fuseNetworkTxHash
    ) external onlyRelayer whenNotPaused {
        require(recipient != address(0), "FuseBridge: zero recipient");
        require(amount > 0,              "FuseBridge: zero amount");
        require(
            !processedTxHashes[fuseNetworkTxHash],
            "FuseBridge: tx already processed"
        );

        processedTxHashes[fuseNetworkTxHash] = true;
        fuseToken.mint(recipient, amount);

        emit TokensMinted(recipient, amount, fuseNetworkTxHash);
    }

    /**
     * @notice Burn wrapped FUSE on Ethereum to unlock native FUSE on Fuse Network.
     * @dev The caller must have approved this contract to spend `amount` of
     *      FuseToken, or the tokens will be transferred directly.
     * @param amount                Amount of FUSE (18 decimals) to bridge back.
     * @param fuseNetworkRecipient  Fuse Network address (0x… or checksummed) of
     *                              the ultimate recipient.
     */
    function burn(
        uint256 amount,
        string calldata fuseNetworkRecipient
    ) external whenNotPaused {
        require(amount > 0,                        "FuseBridge: zero amount");
        require(bytes(fuseNetworkRecipient).length > 0, "FuseBridge: empty recipient");

        fuseToken.burn(msg.sender, amount);

        emit TokensBurned(msg.sender, amount, fuseNetworkRecipient);
    }
}
