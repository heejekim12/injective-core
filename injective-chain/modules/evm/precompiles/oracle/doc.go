// Package oracle provides the Oracle EVM precompile.
// The precompile is deployed at address 0x0000000000000000000000000000000000000067 and exposes
// a single read-only method, oraclePrice, which returns the on-chain reference price for any
// supported oracle type as a 1e18-scaled uint256.
package oracle
