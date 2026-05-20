// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package oracle

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// IOracleModulePricePairState is an auto generated low-level Go binding around an user-defined struct.
type IOracleModulePricePairState struct {
	PairPrice            *big.Int
	BasePrice            *big.Int
	QuotePrice           *big.Int
	BaseCumulativePrice  *big.Int
	QuoteCumulativePrice *big.Int
	BaseTimestamp        uint64
	QuoteTimestamp       uint64
}

// OracleModuleMetaData contains all meta data concerning the OracleModule contract.
var OracleModuleMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"oraclePrice\",\"inputs\":[{\"name\":\"oracleType\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"base\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"quote\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"price\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"oraclePricePairState\",\"inputs\":[{\"name\":\"oracleType\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"base\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"quote\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"state\",\"type\":\"tuple\",\"internalType\":\"structIOracleModule.PricePairState\",\"components\":[{\"name\":\"pairPrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"basePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quotePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quoteCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"quoteTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"oraclePricePairStateScaled\",\"inputs\":[{\"name\":\"oracleType\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"base\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"quote\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"baseDecimals\",\"type\":\"uint32\",\"internalType\":\"uint32\"},{\"name\":\"quoteDecimals\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"outputs\":[{\"name\":\"state\",\"type\":\"tuple\",\"internalType\":\"structIOracleModule.PricePairState\",\"components\":[{\"name\":\"pairPrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"basePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quotePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quoteCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"quoteTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"}]",
}

// OracleModuleABI is the input ABI used to generate the binding from.
// Deprecated: Use OracleModuleMetaData.ABI instead.
var OracleModuleABI = OracleModuleMetaData.ABI

// OracleModule is an auto generated Go binding around an Ethereum contract.
type OracleModule struct {
	OracleModuleCaller     // Read-only binding to the contract
	OracleModuleTransactor // Write-only binding to the contract
	OracleModuleFilterer   // Log filterer for contract events
}

// OracleModuleCaller is an auto generated read-only Go binding around an Ethereum contract.
type OracleModuleCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OracleModuleTransactor is an auto generated write-only Go binding around an Ethereum contract.
type OracleModuleTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OracleModuleFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type OracleModuleFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OracleModuleSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type OracleModuleSession struct {
	Contract     *OracleModule     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// OracleModuleCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type OracleModuleCallerSession struct {
	Contract *OracleModuleCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// OracleModuleTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type OracleModuleTransactorSession struct {
	Contract     *OracleModuleTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// OracleModuleRaw is an auto generated low-level Go binding around an Ethereum contract.
type OracleModuleRaw struct {
	Contract *OracleModule // Generic contract binding to access the raw methods on
}

// OracleModuleCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type OracleModuleCallerRaw struct {
	Contract *OracleModuleCaller // Generic read-only contract binding to access the raw methods on
}

// OracleModuleTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type OracleModuleTransactorRaw struct {
	Contract *OracleModuleTransactor // Generic write-only contract binding to access the raw methods on
}

// NewOracleModule creates a new instance of OracleModule, bound to a specific deployed contract.
func NewOracleModule(address common.Address, backend bind.ContractBackend) (*OracleModule, error) {
	contract, err := bindOracleModule(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &OracleModule{OracleModuleCaller: OracleModuleCaller{contract: contract}, OracleModuleTransactor: OracleModuleTransactor{contract: contract}, OracleModuleFilterer: OracleModuleFilterer{contract: contract}}, nil
}

// NewOracleModuleCaller creates a new read-only instance of OracleModule, bound to a specific deployed contract.
func NewOracleModuleCaller(address common.Address, caller bind.ContractCaller) (*OracleModuleCaller, error) {
	contract, err := bindOracleModule(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &OracleModuleCaller{contract: contract}, nil
}

// NewOracleModuleTransactor creates a new write-only instance of OracleModule, bound to a specific deployed contract.
func NewOracleModuleTransactor(address common.Address, transactor bind.ContractTransactor) (*OracleModuleTransactor, error) {
	contract, err := bindOracleModule(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &OracleModuleTransactor{contract: contract}, nil
}

// NewOracleModuleFilterer creates a new log filterer instance of OracleModule, bound to a specific deployed contract.
func NewOracleModuleFilterer(address common.Address, filterer bind.ContractFilterer) (*OracleModuleFilterer, error) {
	contract, err := bindOracleModule(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &OracleModuleFilterer{contract: contract}, nil
}

// bindOracleModule binds a generic wrapper to an already deployed contract.
func bindOracleModule(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := OracleModuleMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_OracleModule *OracleModuleRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _OracleModule.Contract.OracleModuleCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_OracleModule *OracleModuleRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _OracleModule.Contract.OracleModuleTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_OracleModule *OracleModuleRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _OracleModule.Contract.OracleModuleTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_OracleModule *OracleModuleCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _OracleModule.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_OracleModule *OracleModuleTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _OracleModule.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_OracleModule *OracleModuleTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _OracleModule.Contract.contract.Transact(opts, method, params...)
}

// OraclePrice is a free data retrieval call binding the contract method 0x6a06eb6f.
//
// Solidity: function oraclePrice(uint8 oracleType, string base, string quote) view returns(uint256 price)
func (_OracleModule *OracleModuleCaller) OraclePrice(opts *bind.CallOpts, oracleType uint8, base string, quote string) (*big.Int, error) {
	var out []interface{}
	err := _OracleModule.contract.Call(opts, &out, "oraclePrice", oracleType, base, quote)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// OraclePrice is a free data retrieval call binding the contract method 0x6a06eb6f.
//
// Solidity: function oraclePrice(uint8 oracleType, string base, string quote) view returns(uint256 price)
func (_OracleModule *OracleModuleSession) OraclePrice(oracleType uint8, base string, quote string) (*big.Int, error) {
	return _OracleModule.Contract.OraclePrice(&_OracleModule.CallOpts, oracleType, base, quote)
}

// OraclePrice is a free data retrieval call binding the contract method 0x6a06eb6f.
//
// Solidity: function oraclePrice(uint8 oracleType, string base, string quote) view returns(uint256 price)
func (_OracleModule *OracleModuleCallerSession) OraclePrice(oracleType uint8, base string, quote string) (*big.Int, error) {
	return _OracleModule.Contract.OraclePrice(&_OracleModule.CallOpts, oracleType, base, quote)
}

// OraclePricePairState is a free data retrieval call binding the contract method 0x20374400.
//
// Solidity: function oraclePricePairState(uint8 oracleType, string base, string quote) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleModule *OracleModuleCaller) OraclePricePairState(opts *bind.CallOpts, oracleType uint8, base string, quote string) (IOracleModulePricePairState, error) {
	var out []interface{}
	err := _OracleModule.contract.Call(opts, &out, "oraclePricePairState", oracleType, base, quote)

	if err != nil {
		return *new(IOracleModulePricePairState), err
	}

	out0 := *abi.ConvertType(out[0], new(IOracleModulePricePairState)).(*IOracleModulePricePairState)

	return out0, err

}

// OraclePricePairState is a free data retrieval call binding the contract method 0x20374400.
//
// Solidity: function oraclePricePairState(uint8 oracleType, string base, string quote) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleModule *OracleModuleSession) OraclePricePairState(oracleType uint8, base string, quote string) (IOracleModulePricePairState, error) {
	return _OracleModule.Contract.OraclePricePairState(&_OracleModule.CallOpts, oracleType, base, quote)
}

// OraclePricePairState is a free data retrieval call binding the contract method 0x20374400.
//
// Solidity: function oraclePricePairState(uint8 oracleType, string base, string quote) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleModule *OracleModuleCallerSession) OraclePricePairState(oracleType uint8, base string, quote string) (IOracleModulePricePairState, error) {
	return _OracleModule.Contract.OraclePricePairState(&_OracleModule.CallOpts, oracleType, base, quote)
}

// OraclePricePairStateScaled is a free data retrieval call binding the contract method 0xc24bfb32.
//
// Solidity: function oraclePricePairStateScaled(uint8 oracleType, string base, string quote, uint32 baseDecimals, uint32 quoteDecimals) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleModule *OracleModuleCaller) OraclePricePairStateScaled(opts *bind.CallOpts, oracleType uint8, base string, quote string, baseDecimals uint32, quoteDecimals uint32) (IOracleModulePricePairState, error) {
	var out []interface{}
	err := _OracleModule.contract.Call(opts, &out, "oraclePricePairStateScaled", oracleType, base, quote, baseDecimals, quoteDecimals)

	if err != nil {
		return *new(IOracleModulePricePairState), err
	}

	out0 := *abi.ConvertType(out[0], new(IOracleModulePricePairState)).(*IOracleModulePricePairState)

	return out0, err

}

// OraclePricePairStateScaled is a free data retrieval call binding the contract method 0xc24bfb32.
//
// Solidity: function oraclePricePairStateScaled(uint8 oracleType, string base, string quote, uint32 baseDecimals, uint32 quoteDecimals) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleModule *OracleModuleSession) OraclePricePairStateScaled(oracleType uint8, base string, quote string, baseDecimals uint32, quoteDecimals uint32) (IOracleModulePricePairState, error) {
	return _OracleModule.Contract.OraclePricePairStateScaled(&_OracleModule.CallOpts, oracleType, base, quote, baseDecimals, quoteDecimals)
}

// OraclePricePairStateScaled is a free data retrieval call binding the contract method 0xc24bfb32.
//
// Solidity: function oraclePricePairStateScaled(uint8 oracleType, string base, string quote, uint32 baseDecimals, uint32 quoteDecimals) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleModule *OracleModuleCallerSession) OraclePricePairStateScaled(oracleType uint8, base string, quote string, baseDecimals uint32, quoteDecimals uint32) (IOracleModulePricePairState, error) {
	return _OracleModule.Contract.OraclePricePairStateScaled(&_OracleModule.CallOpts, oracleType, base, quote, baseDecimals, quoteDecimals)
}
