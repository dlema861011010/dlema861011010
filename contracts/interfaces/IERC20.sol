// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @dev Interface of the ERC-20 standard as defined in EIP-20.
 */
interface IERC20 {
    /// @dev Emitted when `value` tokens are moved from `from` to `to`.
    event Transfer(address indexed from, address indexed to, uint256 value);

    /// @dev Emitted when the allowance of a `spender` for an `owner` is set.
    event Approval(address indexed owner, address indexed spender, uint256 value);

    /// @return Total number of tokens in existence.
    function totalSupply() external view returns (uint256);

    /// @return Amount of tokens owned by `account`.
    function balanceOf(address account) external view returns (uint256);

    /**
     * @dev Moves `amount` tokens from the caller's account to `to`.
     * Returns a boolean indicating success.
     */
    function transfer(address to, uint256 amount) external returns (bool);

    /// @return Remaining number of tokens that `spender` is allowed to spend on behalf of `owner`.
    function allowance(address owner, address spender) external view returns (uint256);

    /**
     * @dev Sets `amount` as the allowance of `spender` over the caller's tokens.
     * Returns a boolean indicating success.
     */
    function approve(address spender, uint256 amount) external returns (bool);

    /**
     * @dev Moves `amount` tokens from `from` to `to` using the allowance mechanism.
     * Returns a boolean indicating success.
     */
    function transferFrom(address from, address to, uint256 amount) external returns (bool);
}
