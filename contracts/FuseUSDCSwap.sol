// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./interfaces/IERC20.sol";

/**
 * @title FuseUSDCSwap
 * @notice Trustless on-chain swap helper that allows users to exchange:
 *           • ETH  ↔  FUSE  (wrapped FUSE ERC-20)
 *           • USDC ↔  FUSE
 *
 * The contract uses a constant-product AMM (x * y = k) for each pair and
 * holds reserves of FUSE, ETH (as the contract's native balance), and USDC.
 *
 * Liquidity providers call `addLiquidity*` to deposit assets and receive
 * LP shares.  They call `removeLiquidity*` to reclaim their proportional share.
 *
 * Swap callers pay a 0.3 % fee that stays in the pool (increases reserves).
 *
 * Overflow note: Solidity ^0.8 reverts on overflow automatically.  The initial
 * share calculation (_sqrt(a * b)) therefore reverts for extremely large first
 * deposits (a * b > type(uint256).max).  Practical deposit sizes are far below
 * this limit.
 *
 * ⚠ This is a reference implementation intended for educational purposes.
 *   Production deployments require audits and further hardening.
 */
contract FuseUSDCSwap {
    // -------------------------------------------------------------------------
    // Reentrancy guard
    // -------------------------------------------------------------------------
    uint256 private _reentrancyStatus;
    uint256 private constant _NOT_ENTERED = 1;
    uint256 private constant _ENTERED     = 2;

    modifier nonReentrant() {
        require(_reentrancyStatus != _ENTERED, "FuseUSDCSwap: reentrant call");
        _reentrancyStatus = _ENTERED;
        _;
        _reentrancyStatus = _NOT_ENTERED;
    }
    // -------------------------------------------------------------------------
    // Constants
    // -------------------------------------------------------------------------
    uint256 public constant FEE_NUMERATOR   = 997; // 0.3 % fee → multiply by 997/1000
    uint256 public constant FEE_DENOMINATOR = 1000;

    // -------------------------------------------------------------------------
    // State
    // -------------------------------------------------------------------------
    address public owner;

    IERC20 public immutable fuseToken;
    IERC20 public immutable usdcToken;

    // --- ETH/FUSE pool ---
    uint256 public ethFuseReserveETH;   // ETH held in pool (wei)
    uint256 public ethFuseReserveFUSE;  // FUSE held in pool
    uint256 public ethFuseTotalShares;
    mapping(address => uint256) public ethFuseShares;

    // --- USDC/FUSE pool ---
    uint256 public usdcFuseReserveUSDC; // USDC held in pool (6 decimals)
    uint256 public usdcFuseReserveFUSE; // FUSE held in pool (18 decimals)
    uint256 public usdcFuseTotalShares;
    mapping(address => uint256) public usdcFuseShares;

    // -------------------------------------------------------------------------
    // Events
    // -------------------------------------------------------------------------
    event LiquidityAddedETHFUSE(
        address indexed provider,
        uint256 ethAmount,
        uint256 fuseAmount,
        uint256 shares
    );
    event LiquidityRemovedETHFUSE(
        address indexed provider,
        uint256 ethAmount,
        uint256 fuseAmount,
        uint256 shares
    );
    event LiquidityAddedUSDCFUSE(
        address indexed provider,
        uint256 usdcAmount,
        uint256 fuseAmount,
        uint256 shares
    );
    event LiquidityRemovedUSDCFUSE(
        address indexed provider,
        uint256 usdcAmount,
        uint256 fuseAmount,
        uint256 shares
    );
    event SwapETHForFUSE(
        address indexed sender,
        uint256 ethIn,
        uint256 fuseOut
    );
    event SwapFUSEForETH(
        address indexed sender,
        uint256 fuseIn,
        uint256 ethOut
    );
    event SwapUSDCForFUSE(
        address indexed sender,
        uint256 usdcIn,
        uint256 fuseOut
    );
    event SwapFUSEForUSDC(
        address indexed sender,
        uint256 fuseIn,
        uint256 usdcOut
    );

    // -------------------------------------------------------------------------
    // Modifiers
    // -------------------------------------------------------------------------
    modifier onlyOwner() {
        require(msg.sender == owner, "FuseUSDCSwap: not owner");
        _;
    }

    // -------------------------------------------------------------------------
    // Constructor
    // -------------------------------------------------------------------------
    /**
     * @param _fuseToken  Address of the FUSE ERC-20 token on Ethereum.
     * @param _usdcToken  Address of the USDC ERC-20 token on Ethereum.
     */
    constructor(address _fuseToken, address _usdcToken) {
        require(_fuseToken != address(0) && _usdcToken != address(0), "FuseUSDCSwap: zero address");
        owner             = msg.sender;
        fuseToken         = IERC20(_fuseToken);
        usdcToken         = IERC20(_usdcToken);
        _reentrancyStatus = _NOT_ENTERED;
    }

    // =========================================================================
    // ETH / FUSE pool
    // =========================================================================

    /**
     * @notice Deposit ETH and FUSE into the ETH/FUSE pool.
     * @dev Caller must approve this contract to spend `fuseAmount` of FUSE.
     *      Send ETH as msg.value; provide matching `fuseAmount`.
     *      First deposit sets the initial price.
     * @param fuseAmount FUSE tokens to deposit (must match the current ratio for
     *                   subsequent deposits).
     */
    function addLiquidityETHFUSE(uint256 fuseAmount) external payable {
        require(msg.value > 0 && fuseAmount > 0, "FuseUSDCSwap: zero amounts");

        uint256 shares;
        if (ethFuseTotalShares == 0) {
            // Initial deposit – geometric mean of the two amounts as shares.
            shares = _sqrt(msg.value * fuseAmount);
        } else {
            // Proportional deposit.
            uint256 sharesFromETH  = (msg.value * ethFuseTotalShares) / ethFuseReserveETH;
            uint256 sharesFromFUSE = (fuseAmount * ethFuseTotalShares) / ethFuseReserveFUSE;
            shares = sharesFromETH < sharesFromFUSE ? sharesFromETH : sharesFromFUSE;
        }

        require(shares > 0, "FuseUSDCSwap: insufficient shares");

        fuseToken.transferFrom(msg.sender, address(this), fuseAmount);

        ethFuseReserveETH  += msg.value;
        ethFuseReserveFUSE += fuseAmount;
        ethFuseTotalShares += shares;
        ethFuseShares[msg.sender] += shares;

        emit LiquidityAddedETHFUSE(msg.sender, msg.value, fuseAmount, shares);
    }

    /**
     * @notice Withdraw ETH and FUSE from the ETH/FUSE pool proportionally.
     * @param shares Number of LP shares to redeem.
     */
    function removeLiquidityETHFUSE(uint256 shares) external nonReentrant {
        require(shares > 0,                         "FuseUSDCSwap: zero shares");
        require(ethFuseShares[msg.sender] >= shares, "FuseUSDCSwap: insufficient shares");

        uint256 ethOut  = (shares * ethFuseReserveETH)  / ethFuseTotalShares;
        uint256 fuseOut = (shares * ethFuseReserveFUSE) / ethFuseTotalShares;

        ethFuseShares[msg.sender] -= shares;
        ethFuseTotalShares        -= shares;
        ethFuseReserveETH         -= ethOut;
        ethFuseReserveFUSE        -= fuseOut;

        fuseToken.transfer(msg.sender, fuseOut);
        (bool ok, ) = msg.sender.call{value: ethOut}("");
        require(ok, "FuseUSDCSwap: ETH transfer failed");

        emit LiquidityRemovedETHFUSE(msg.sender, ethOut, fuseOut, shares);
    }

    /**
     * @notice Swap ETH for FUSE.
     * @param minFuseOut Minimum FUSE to receive (slippage guard).
     */
    function swapETHForFUSE(uint256 minFuseOut) external payable nonReentrant {
        require(msg.value > 0, "FuseUSDCSwap: zero ETH");

        uint256 fuseOut = _getAmountOut(
            msg.value,
            ethFuseReserveETH,
            ethFuseReserveFUSE
        );
        require(fuseOut >= minFuseOut, "FuseUSDCSwap: slippage");

        ethFuseReserveETH  += msg.value;
        ethFuseReserveFUSE -= fuseOut;

        fuseToken.transfer(msg.sender, fuseOut);
        emit SwapETHForFUSE(msg.sender, msg.value, fuseOut);
    }

    /**
     * @notice Swap FUSE for ETH.
     * @dev Caller must approve this contract to spend `fuseIn` of FUSE.
     * @param fuseIn    FUSE to swap.
     * @param minEthOut Minimum ETH to receive (slippage guard).
     */
    function swapFUSEForETH(uint256 fuseIn, uint256 minEthOut) external nonReentrant {
        require(fuseIn > 0, "FuseUSDCSwap: zero FUSE");

        uint256 ethOut = _getAmountOut(
            fuseIn,
            ethFuseReserveFUSE,
            ethFuseReserveETH
        );
        require(ethOut >= minEthOut, "FuseUSDCSwap: slippage");

        fuseToken.transferFrom(msg.sender, address(this), fuseIn);
        ethFuseReserveFUSE += fuseIn;
        ethFuseReserveETH  -= ethOut;

        (bool ok, ) = msg.sender.call{value: ethOut}("");
        require(ok, "FuseUSDCSwap: ETH transfer failed");
        emit SwapFUSEForETH(msg.sender, fuseIn, ethOut);
    }

    // =========================================================================
    // USDC / FUSE pool
    // =========================================================================

    /**
     * @notice Deposit USDC and FUSE into the USDC/FUSE pool.
     * @dev Caller must approve this contract to spend both tokens.
     * @param usdcAmount USDC to deposit (6 decimals).
     * @param fuseAmount FUSE to deposit (18 decimals).
     */
    function addLiquidityUSDCFUSE(uint256 usdcAmount, uint256 fuseAmount) external {
        require(usdcAmount > 0 && fuseAmount > 0, "FuseUSDCSwap: zero amounts");

        uint256 shares;
        if (usdcFuseTotalShares == 0) {
            shares = _sqrt(usdcAmount * fuseAmount);
        } else {
            uint256 sharesFromUSDC = (usdcAmount * usdcFuseTotalShares) / usdcFuseReserveUSDC;
            uint256 sharesFromFUSE = (fuseAmount * usdcFuseTotalShares) / usdcFuseReserveFUSE;
            shares = sharesFromUSDC < sharesFromFUSE ? sharesFromUSDC : sharesFromFUSE;
        }

        require(shares > 0, "FuseUSDCSwap: insufficient shares");

        usdcToken.transferFrom(msg.sender, address(this), usdcAmount);
        fuseToken.transferFrom(msg.sender, address(this), fuseAmount);

        usdcFuseReserveUSDC  += usdcAmount;
        usdcFuseReserveFUSE  += fuseAmount;
        usdcFuseTotalShares  += shares;
        usdcFuseShares[msg.sender] += shares;

        emit LiquidityAddedUSDCFUSE(msg.sender, usdcAmount, fuseAmount, shares);
    }

    /**
     * @notice Withdraw USDC and FUSE from the USDC/FUSE pool proportionally.
     * @param shares Number of LP shares to redeem.
     */
    function removeLiquidityUSDCFUSE(uint256 shares) external {
        require(shares > 0,                          "FuseUSDCSwap: zero shares");
        require(usdcFuseShares[msg.sender] >= shares, "FuseUSDCSwap: insufficient shares");

        uint256 usdcOut = (shares * usdcFuseReserveUSDC) / usdcFuseTotalShares;
        uint256 fuseOut = (shares * usdcFuseReserveFUSE) / usdcFuseTotalShares;

        usdcFuseShares[msg.sender] -= shares;
        usdcFuseTotalShares        -= shares;
        usdcFuseReserveUSDC        -= usdcOut;
        usdcFuseReserveFUSE        -= fuseOut;

        usdcToken.transfer(msg.sender, usdcOut);
        fuseToken.transfer(msg.sender, fuseOut);

        emit LiquidityRemovedUSDCFUSE(msg.sender, usdcOut, fuseOut, shares);
    }

    /**
     * @notice Swap USDC for FUSE.
     * @dev Caller must approve this contract to spend `usdcIn`.
     * @param usdcIn     USDC to swap (6 decimals).
     * @param minFuseOut Minimum FUSE to receive (slippage guard).
     */
    function swapUSDCForFUSE(uint256 usdcIn, uint256 minFuseOut) external {
        require(usdcIn > 0, "FuseUSDCSwap: zero USDC");

        uint256 fuseOut = _getAmountOut(
            usdcIn,
            usdcFuseReserveUSDC,
            usdcFuseReserveFUSE
        );
        require(fuseOut >= minFuseOut, "FuseUSDCSwap: slippage");

        usdcToken.transferFrom(msg.sender, address(this), usdcIn);
        usdcFuseReserveUSDC += usdcIn;
        usdcFuseReserveFUSE -= fuseOut;

        fuseToken.transfer(msg.sender, fuseOut);
        emit SwapUSDCForFUSE(msg.sender, usdcIn, fuseOut);
    }

    /**
     * @notice Swap FUSE for USDC.
     * @dev Caller must approve this contract to spend `fuseIn`.
     * @param fuseIn     FUSE to swap (18 decimals).
     * @param minUsdcOut Minimum USDC to receive (slippage guard).
     */
    function swapFUSEForUSDC(uint256 fuseIn, uint256 minUsdcOut) external {
        require(fuseIn > 0, "FuseUSDCSwap: zero FUSE");

        uint256 usdcOut = _getAmountOut(
            fuseIn,
            usdcFuseReserveFUSE,
            usdcFuseReserveUSDC
        );
        require(usdcOut >= minUsdcOut, "FuseUSDCSwap: slippage");

        fuseToken.transferFrom(msg.sender, address(this), fuseIn);
        usdcFuseReserveFUSE  += fuseIn;
        usdcFuseReserveUSDC  -= usdcOut;

        usdcToken.transfer(msg.sender, usdcOut);
        emit SwapFUSEForUSDC(msg.sender, fuseIn, usdcOut);
    }

    // =========================================================================
    // Internal helpers
    // =========================================================================

    /**
     * @dev Constant-product AMM output amount with 0.3 % fee applied to the input.
     *      amountOut = (amountIn * 997 * reserveOut) / (reserveIn * 1000 + amountIn * 997)
     */
    function _getAmountOut(
        uint256 amountIn,
        uint256 reserveIn,
        uint256 reserveOut
    ) internal pure returns (uint256) {
        require(reserveIn > 0 && reserveOut > 0, "FuseUSDCSwap: empty reserves");
        uint256 amountInWithFee = amountIn * FEE_NUMERATOR;
        uint256 numerator       = amountInWithFee * reserveOut;
        uint256 denominator     = (reserveIn * FEE_DENOMINATOR) + amountInWithFee;
        return numerator / denominator;
    }

    /// @dev Integer square root (Babylonian method).
    function _sqrt(uint256 y) internal pure returns (uint256 z) {
        if (y > 3) {
            z = y;
            uint256 x = y / 2 + 1;
            while (x < z) {
                z = x;
                x = (y / x + x) / 2;
            }
        } else if (y != 0) {
            z = 1;
        }
    }

    /// @dev Accept plain ETH transfers (e.g., from removeLiquidity refunds).
    receive() external payable {}
}
