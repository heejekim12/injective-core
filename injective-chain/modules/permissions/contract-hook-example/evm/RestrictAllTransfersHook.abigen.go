// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package evm

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

// RestrictAllTransfersHookMetaData contains all meta data concerning the RestrictAllTransfersHook contract.
var RestrictAllTransfersHookMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"isTransferRestricted\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"tuple\",\"internalType\":\"structCosmos.Coin\",\"components\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"denom\",\"type\":\"string\",\"internalType\":\"string\"}]}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"pure\"}]",
	Bin: "0x6080604052348015600e575f5ffd5b506101c28061001c5f395ff3fe608060405234801561000f575f5ffd5b5060043610610029575f3560e01c8063e4e69baf1461002d575b5f5ffd5b610047600480360381019061004291906100ed565b61005d565b6040516100549190610173565b60405180910390f35b5f600190509392505050565b5f5ffd5b5f5ffd5b5f73ffffffffffffffffffffffffffffffffffffffff82169050919050565b5f61009a82610071565b9050919050565b6100aa81610090565b81146100b4575f5ffd5b50565b5f813590506100c5816100a1565b92915050565b5f5ffd5b5f604082840312156100e4576100e36100cb565b5b81905092915050565b5f5f5f6060848603121561010457610103610069565b5b5f610111868287016100b7565b9350506020610122868287016100b7565b925050604084013567ffffffffffffffff8111156101435761014261006d565b5b61014f868287016100cf565b9150509250925092565b5f8115159050919050565b61016d81610159565b82525050565b5f6020820190506101865f830184610164565b9291505056fea2646970667358221220aff286bff2d53783283544dfe4edb9b387df15ae98aa0b52c04a43db8184082e64736f6c634300081e0033",
}

// RestrictAllTransfersHookABI is the input ABI used to generate the binding from.
// Deprecated: Use RestrictAllTransfersHookMetaData.ABI instead.
var RestrictAllTransfersHookABI = RestrictAllTransfersHookMetaData.ABI

// RestrictAllTransfersHookBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use RestrictAllTransfersHookMetaData.Bin instead.
var RestrictAllTransfersHookBin = RestrictAllTransfersHookMetaData.Bin

// DeployRestrictAllTransfersHook deploys a new Ethereum contract, binding an instance of RestrictAllTransfersHook to it.
func DeployRestrictAllTransfersHook(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *RestrictAllTransfersHook, error) {
	parsed, err := RestrictAllTransfersHookMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(RestrictAllTransfersHookBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &RestrictAllTransfersHook{RestrictAllTransfersHookCaller: RestrictAllTransfersHookCaller{contract: contract}, RestrictAllTransfersHookTransactor: RestrictAllTransfersHookTransactor{contract: contract}, RestrictAllTransfersHookFilterer: RestrictAllTransfersHookFilterer{contract: contract}}, nil
}

// RestrictAllTransfersHook is an auto generated Go binding around an Ethereum contract.
type RestrictAllTransfersHook struct {
	RestrictAllTransfersHookCaller     // Read-only binding to the contract
	RestrictAllTransfersHookTransactor // Write-only binding to the contract
	RestrictAllTransfersHookFilterer   // Log filterer for contract events
}

// RestrictAllTransfersHookCaller is an auto generated read-only Go binding around an Ethereum contract.
type RestrictAllTransfersHookCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RestrictAllTransfersHookTransactor is an auto generated write-only Go binding around an Ethereum contract.
type RestrictAllTransfersHookTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RestrictAllTransfersHookFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type RestrictAllTransfersHookFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RestrictAllTransfersHookSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type RestrictAllTransfersHookSession struct {
	Contract     *RestrictAllTransfersHook // Generic contract binding to set the session for
	CallOpts     bind.CallOpts             // Call options to use throughout this session
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// RestrictAllTransfersHookCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type RestrictAllTransfersHookCallerSession struct {
	Contract *RestrictAllTransfersHookCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                   // Call options to use throughout this session
}

// RestrictAllTransfersHookTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type RestrictAllTransfersHookTransactorSession struct {
	Contract     *RestrictAllTransfersHookTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                   // Transaction auth options to use throughout this session
}

// RestrictAllTransfersHookRaw is an auto generated low-level Go binding around an Ethereum contract.
type RestrictAllTransfersHookRaw struct {
	Contract *RestrictAllTransfersHook // Generic contract binding to access the raw methods on
}

// RestrictAllTransfersHookCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type RestrictAllTransfersHookCallerRaw struct {
	Contract *RestrictAllTransfersHookCaller // Generic read-only contract binding to access the raw methods on
}

// RestrictAllTransfersHookTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type RestrictAllTransfersHookTransactorRaw struct {
	Contract *RestrictAllTransfersHookTransactor // Generic write-only contract binding to access the raw methods on
}

// NewRestrictAllTransfersHook creates a new instance of RestrictAllTransfersHook, bound to a specific deployed contract.
func NewRestrictAllTransfersHook(address common.Address, backend bind.ContractBackend) (*RestrictAllTransfersHook, error) {
	contract, err := bindRestrictAllTransfersHook(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &RestrictAllTransfersHook{RestrictAllTransfersHookCaller: RestrictAllTransfersHookCaller{contract: contract}, RestrictAllTransfersHookTransactor: RestrictAllTransfersHookTransactor{contract: contract}, RestrictAllTransfersHookFilterer: RestrictAllTransfersHookFilterer{contract: contract}}, nil
}

// NewRestrictAllTransfersHookCaller creates a new read-only instance of RestrictAllTransfersHook, bound to a specific deployed contract.
func NewRestrictAllTransfersHookCaller(address common.Address, caller bind.ContractCaller) (*RestrictAllTransfersHookCaller, error) {
	contract, err := bindRestrictAllTransfersHook(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &RestrictAllTransfersHookCaller{contract: contract}, nil
}

// NewRestrictAllTransfersHookTransactor creates a new write-only instance of RestrictAllTransfersHook, bound to a specific deployed contract.
func NewRestrictAllTransfersHookTransactor(address common.Address, transactor bind.ContractTransactor) (*RestrictAllTransfersHookTransactor, error) {
	contract, err := bindRestrictAllTransfersHook(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &RestrictAllTransfersHookTransactor{contract: contract}, nil
}

// NewRestrictAllTransfersHookFilterer creates a new log filterer instance of RestrictAllTransfersHook, bound to a specific deployed contract.
func NewRestrictAllTransfersHookFilterer(address common.Address, filterer bind.ContractFilterer) (*RestrictAllTransfersHookFilterer, error) {
	contract, err := bindRestrictAllTransfersHook(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &RestrictAllTransfersHookFilterer{contract: contract}, nil
}

// bindRestrictAllTransfersHook binds a generic wrapper to an already deployed contract.
func bindRestrictAllTransfersHook(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := RestrictAllTransfersHookMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_RestrictAllTransfersHook *RestrictAllTransfersHookRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _RestrictAllTransfersHook.Contract.RestrictAllTransfersHookCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_RestrictAllTransfersHook *RestrictAllTransfersHookRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _RestrictAllTransfersHook.Contract.RestrictAllTransfersHookTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_RestrictAllTransfersHook *RestrictAllTransfersHookRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _RestrictAllTransfersHook.Contract.RestrictAllTransfersHookTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_RestrictAllTransfersHook *RestrictAllTransfersHookCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _RestrictAllTransfersHook.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_RestrictAllTransfersHook *RestrictAllTransfersHookTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _RestrictAllTransfersHook.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_RestrictAllTransfersHook *RestrictAllTransfersHookTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _RestrictAllTransfersHook.Contract.contract.Transact(opts, method, params...)
}

// IsTransferRestricted is a free data retrieval call binding the contract method 0xe4e69baf.
//
// Solidity: function isTransferRestricted(address from, address to, (uint256,string) amount) pure returns(bool)
func (_RestrictAllTransfersHook *RestrictAllTransfersHookCaller) IsTransferRestricted(opts *bind.CallOpts, from common.Address, to common.Address, amount CosmosCoin) (bool, error) {
	var out []interface{}
	err := _RestrictAllTransfersHook.contract.Call(opts, &out, "isTransferRestricted", from, to, amount)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsTransferRestricted is a free data retrieval call binding the contract method 0xe4e69baf.
//
// Solidity: function isTransferRestricted(address from, address to, (uint256,string) amount) pure returns(bool)
func (_RestrictAllTransfersHook *RestrictAllTransfersHookSession) IsTransferRestricted(from common.Address, to common.Address, amount CosmosCoin) (bool, error) {
	return _RestrictAllTransfersHook.Contract.IsTransferRestricted(&_RestrictAllTransfersHook.CallOpts, from, to, amount)
}

// IsTransferRestricted is a free data retrieval call binding the contract method 0xe4e69baf.
//
// Solidity: function isTransferRestricted(address from, address to, (uint256,string) amount) pure returns(bool)
func (_RestrictAllTransfersHook *RestrictAllTransfersHookCallerSession) IsTransferRestricted(from common.Address, to common.Address, amount CosmosCoin) (bool, error) {
	return _RestrictAllTransfersHook.Contract.IsTransferRestricted(&_RestrictAllTransfersHook.CallOpts, from, to, amount)
}
