// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./interfaces/IERC20.sol";

/**
 * @title FuseToken
 * @notice ERC-20 representation of the Fuse Network native token (FUSE) on Ethereum.
 *
 * Only a designated minter (typically the FuseBridge contract) may mint or burn
 * tokens, keeping the supply on Ethereum in sync with locked FUSE on the Fuse Network.
 */
contract FuseToken is IERC20 {
    // -------------------------------------------------------------------------
    // Metadata
    // -------------------------------------------------------------------------
    string public constant name     = "Fuse Token";
    string public constant symbol   = "FUSE";
    uint8  public constant decimals = 18;

    // -------------------------------------------------------------------------
    // State
    // -------------------------------------------------------------------------
    address public owner;
    address public minter; // bridge contract address

    uint256 private _totalSupply;
    mapping(address => uint256) private _balances;
    mapping(address => mapping(address => uint256)) private _allowances;

    // -------------------------------------------------------------------------
    // Events
    // -------------------------------------------------------------------------
    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);
    event MinterChanged(address indexed previousMinter, address indexed newMinter);
    event Mint(address indexed to, uint256 amount);
    event Burn(address indexed from, uint256 amount);

    // -------------------------------------------------------------------------
    // Modifiers
    // -------------------------------------------------------------------------
    modifier onlyOwner() {
        require(msg.sender == owner, "FuseToken: caller is not owner");
        _;
    }

    modifier onlyMinter() {
        require(msg.sender == minter, "FuseToken: caller is not minter");
        _;
    }

    // -------------------------------------------------------------------------
    // Constructor
    // -------------------------------------------------------------------------
    constructor() {
        owner = msg.sender;
    }

    // -------------------------------------------------------------------------
    // Administration
    // -------------------------------------------------------------------------

    /**
     * @notice Transfer contract ownership.
     * @param newOwner Address of the new owner.
     */
    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "FuseToken: zero address");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    /**
     * @notice Set the minter address (typically the bridge contract).
     * @param newMinter Address allowed to mint and burn tokens.
     */
    function setMinter(address newMinter) external onlyOwner {
        require(newMinter != address(0), "FuseToken: zero address");
        emit MinterChanged(minter, newMinter);
        minter = newMinter;
    }

    // -------------------------------------------------------------------------
    // Mint / Burn (bridge operations)
    // -------------------------------------------------------------------------

    /**
     * @notice Mint `amount` FUSE tokens to `to`.
     * @dev Called by the bridge when FUSE is locked on the Fuse Network side.
     * @param to     Recipient address.
     * @param amount Amount of tokens (18 decimals).
     */
    function mint(address to, uint256 amount) external onlyMinter {
        require(to != address(0), "FuseToken: mint to zero address");
        _totalSupply += amount;
        _balances[to] += amount;
        emit Transfer(address(0), to, amount);
        emit Mint(to, amount);
    }

    /**
     * @notice Burn `amount` FUSE tokens from `from`.
     * @dev Called by the bridge when the user wants to move FUSE back to Fuse Network.
     * @param from   Address to burn from.
     * @param amount Amount of tokens (18 decimals).
     */
    function burn(address from, uint256 amount) external onlyMinter {
        require(_balances[from] >= amount, "FuseToken: burn exceeds balance");
        _balances[from] -= amount;
        _totalSupply     -= amount;
        emit Transfer(from, address(0), amount);
        emit Burn(from, amount);
    }

    // -------------------------------------------------------------------------
    // ERC-20 view functions
    // -------------------------------------------------------------------------

    function totalSupply() external view override returns (uint256) {
        return _totalSupply;
    }

    function balanceOf(address account) external view override returns (uint256) {
        return _balances[account];
    }

    function allowance(address _owner, address spender) external view override returns (uint256) {
        return _allowances[_owner][spender];
    }

    // -------------------------------------------------------------------------
    // ERC-20 state-changing functions
    // -------------------------------------------------------------------------

    function transfer(address to, uint256 amount) external override returns (bool) {
        _transfer(msg.sender, to, amount);
        return true;
    }

    function approve(address spender, uint256 amount) external override returns (bool) {
        _approve(msg.sender, spender, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) external override returns (bool) {
        uint256 currentAllowance = _allowances[from][msg.sender];
        require(currentAllowance >= amount, "FuseToken: insufficient allowance");
        unchecked { _allowances[from][msg.sender] = currentAllowance - amount; }
        _transfer(from, to, amount);
        return true;
    }

    // -------------------------------------------------------------------------
    // Internal helpers
    // -------------------------------------------------------------------------

    function _transfer(address from, address to, uint256 amount) internal {
        require(from != address(0), "FuseToken: transfer from zero address");
        require(to   != address(0), "FuseToken: transfer to zero address");
        require(_balances[from] >= amount, "FuseToken: transfer exceeds balance");
        unchecked {
            _balances[from] -= amount;
        }
        _balances[to] += amount;
        emit Transfer(from, to, amount);
    }

    function _approve(address _owner, address spender, uint256 amount) internal {
        require(_owner   != address(0), "FuseToken: approve from zero address");
        require(spender  != address(0), "FuseToken: approve to zero address");
        _allowances[_owner][spender] = amount;
        emit Approval(_owner, spender, amount);
    }
}
