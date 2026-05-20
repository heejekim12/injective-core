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

// CosmosCoin is an auto generated low-level Go binding around an user-defined struct.
type CosmosCoin struct {
	Amount *big.Int
	Denom  string
}

// RestrictSpecificAddressTransferHookMetaData contains all meta data concerning the RestrictSpecificAddressTransferHook contract.
var RestrictSpecificAddressTransferHookMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"isTransferRestricted\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"tuple\",\"internalType\":\"structCosmos.Coin\",\"components\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"denom\",\"type\":\"string\",\"internalType\":\"string\"}]}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"pure\"},{\"type\":\"error\",\"name\":\"ArtificialRevert\",\"inputs\":[]}]",
	Bin: "0x6080604052348015600e575f5ffd5b506103bb8061001c5f395ff3fe608060405234801561000f575f5ffd5b5060043610610029575f3560e01c8063e4e69baf1461002d575b5f5ffd5b610047600480360381019061004291906102e6565b61005d565b604051610054919061036c565b60405180910390f35b5f73963ebdf2e1f8db8707d05fc75bfeffba1b5bac1773ffffffffffffffffffffffffffffffffffffffff168473ffffffffffffffffffffffffffffffffffffffff1614806100eb575073963ebdf2e1f8db8707d05fc75bfeffba1b5bac1773ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff16145b156100f9576001905061025b565b736880d7bfe96d49501141375ed835c24cf70e2bd773ffffffffffffffffffffffffffffffffffffffff168473ffffffffffffffffffffffffffffffffffffffff1614806101865750736880d7bfe96d49501141375ed835c24cf70e2bd773ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff16145b15610193575b600161018c575b73727aee334987c52fa7b567b2662bdbb68614e48c73ffffffffffffffffffffffffffffffffffffffff168473ffffffffffffffffffffffffffffffffffffffff161480610220575073727aee334987c52fa7b567b2662bdbb68614e48c73ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff16145b15610257576040517fc3538d2e00000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5f90505b9392505050565b5f5ffd5b5f5ffd5b5f73ffffffffffffffffffffffffffffffffffffffff82169050919050565b5f6102938261026a565b9050919050565b6102a381610289565b81146102ad575f5ffd5b50565b5f813590506102be8161029a565b92915050565b5f5ffd5b5f604082840312156102dd576102dc6102c4565b5b81905092915050565b5f5f5f606084860312156102fd576102fc610262565b5b5f61030a868287016102b0565b935050602061031b868287016102b0565b925050604084013567ffffffffffffffff81111561033c5761033b610266565b5b610348868287016102c8565b9150509250925092565b5f8115159050919050565b61036681610352565b82525050565b5f60208201905061037f5f83018461035d565b9291505056fea2646970667358221220e0069a103010127a18a980bfbd1a123a480f200ed8e88f25f0737c0531665f6564736f6c634300081e0033",
}

// RestrictSpecificAddressTransferHookABI is the input ABI used to generate the binding from.
// Deprecated: Use RestrictSpecificAddressTransferHookMetaData.ABI instead.
var RestrictSpecificAddressTransferHookABI = RestrictSpecificAddressTransferHookMetaData.ABI

// RestrictSpecificAddressTransferHookBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use RestrictSpecificAddressTransferHookMetaData.Bin instead.
var RestrictSpecificAddressTransferHookBin = RestrictSpecificAddressTransferHookMetaData.Bin

// DeployRestrictSpecificAddressTransferHook deploys a new Ethereum contract, binding an instance of RestrictSpecificAddressTransferHook to it.
func DeployRestrictSpecificAddressTransferHook(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *RestrictSpecificAddressTransferHook, error) {
	parsed, err := RestrictSpecificAddressTransferHookMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(RestrictSpecificAddressTransferHookBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &RestrictSpecificAddressTransferHook{RestrictSpecificAddressTransferHookCaller: RestrictSpecificAddressTransferHookCaller{contract: contract}, RestrictSpecificAddressTransferHookTransactor: RestrictSpecificAddressTransferHookTransactor{contract: contract}, RestrictSpecificAddressTransferHookFilterer: RestrictSpecificAddressTransferHookFilterer{contract: contract}}, nil
}

// RestrictSpecificAddressTransferHook is an auto generated Go binding around an Ethereum contract.
type RestrictSpecificAddressTransferHook struct {
	RestrictSpecificAddressTransferHookCaller     // Read-only binding to the contract
	RestrictSpecificAddressTransferHookTransactor // Write-only binding to the contract
	RestrictSpecificAddressTransferHookFilterer   // Log filterer for contract events
}

// RestrictSpecificAddressTransferHookCaller is an auto generated read-only Go binding around an Ethereum contract.
type RestrictSpecificAddressTransferHookCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RestrictSpecificAddressTransferHookTransactor is an auto generated write-only Go binding around an Ethereum contract.
type RestrictSpecificAddressTransferHookTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RestrictSpecificAddressTransferHookFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type RestrictSpecificAddressTransferHookFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RestrictSpecificAddressTransferHookSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type RestrictSpecificAddressTransferHookSession struct {
	Contract     *RestrictSpecificAddressTransferHook // Generic contract binding to set the session for
	CallOpts     bind.CallOpts                        // Call options to use throughout this session
	TransactOpts bind.TransactOpts                    // Transaction auth options to use throughout this session
}

// RestrictSpecificAddressTransferHookCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type RestrictSpecificAddressTransferHookCallerSession struct {
	Contract *RestrictSpecificAddressTransferHookCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                              // Call options to use throughout this session
}

// RestrictSpecificAddressTransferHookTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type RestrictSpecificAddressTransferHookTransactorSession struct {
	Contract     *RestrictSpecificAddressTransferHookTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                              // Transaction auth options to use throughout this session
}

// RestrictSpecificAddressTransferHookRaw is an auto generated low-level Go binding around an Ethereum contract.
type RestrictSpecificAddressTransferHookRaw struct {
	Contract *RestrictSpecificAddressTransferHook // Generic contract binding to access the raw methods on
}

// RestrictSpecificAddressTransferHookCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type RestrictSpecificAddressTransferHookCallerRaw struct {
	Contract *RestrictSpecificAddressTransferHookCaller // Generic read-only contract binding to access the raw methods on
}

// RestrictSpecificAddressTransferHookTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type RestrictSpecificAddressTransferHookTransactorRaw struct {
	Contract *RestrictSpecificAddressTransferHookTransactor // Generic write-only contract binding to access the raw methods on
}

// NewRestrictSpecificAddressTransferHook creates a new instance of RestrictSpecificAddressTransferHook, bound to a specific deployed contract.
func NewRestrictSpecificAddressTransferHook(address common.Address, backend bind.ContractBackend) (*RestrictSpecificAddressTransferHook, error) {
	contract, err := bindRestrictSpecificAddressTransferHook(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &RestrictSpecificAddressTransferHook{RestrictSpecificAddressTransferHookCaller: RestrictSpecificAddressTransferHookCaller{contract: contract}, RestrictSpecificAddressTransferHookTransactor: RestrictSpecificAddressTransferHookTransactor{contract: contract}, RestrictSpecificAddressTransferHookFilterer: RestrictSpecificAddressTransferHookFilterer{contract: contract}}, nil
}

// NewRestrictSpecificAddressTransferHookCaller creates a new read-only instance of RestrictSpecificAddressTransferHook, bound to a specific deployed contract.
func NewRestrictSpecificAddressTransferHookCaller(address common.Address, caller bind.ContractCaller) (*RestrictSpecificAddressTransferHookCaller, error) {
	contract, err := bindRestrictSpecificAddressTransferHook(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &RestrictSpecificAddressTransferHookCaller{contract: contract}, nil
}

// NewRestrictSpecificAddressTransferHookTransactor creates a new write-only instance of RestrictSpecificAddressTransferHook, bound to a specific deployed contract.
func NewRestrictSpecificAddressTransferHookTransactor(address common.Address, transactor bind.ContractTransactor) (*RestrictSpecificAddressTransferHookTransactor, error) {
	contract, err := bindRestrictSpecificAddressTransferHook(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &RestrictSpecificAddressTransferHookTransactor{contract: contract}, nil
}

// NewRestrictSpecificAddressTransferHookFilterer creates a new log filterer instance of RestrictSpecificAddressTransferHook, bound to a specific deployed contract.
func NewRestrictSpecificAddressTransferHookFilterer(address common.Address, filterer bind.ContractFilterer) (*RestrictSpecificAddressTransferHookFilterer, error) {
	contract, err := bindRestrictSpecificAddressTransferHook(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &RestrictSpecificAddressTransferHookFilterer{contract: contract}, nil
}

// bindRestrictSpecificAddressTransferHook binds a generic wrapper to an already deployed contract.
func bindRestrictSpecificAddressTransferHook(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := RestrictSpecificAddressTransferHookMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_RestrictSpecificAddressTransferHook *RestrictSpecificAddressTransferHookRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _RestrictSpecificAddressTransferHook.Contract.RestrictSpecificAddressTransferHookCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_RestrictSpecificAddressTransferHook *RestrictSpecificAddressTransferHookRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _RestrictSpecificAddressTransferHook.Contract.RestrictSpecificAddressTransferHookTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_RestrictSpecificAddressTransferHook *RestrictSpecificAddressTransferHookRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _RestrictSpecificAddressTransferHook.Contract.RestrictSpecificAddressTransferHookTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_RestrictSpecificAddressTransferHook *RestrictSpecificAddressTransferHookCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _RestrictSpecificAddressTransferHook.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_RestrictSpecificAddressTransferHook *RestrictSpecificAddressTransferHookTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _RestrictSpecificAddressTransferHook.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_RestrictSpecificAddressTransferHook *RestrictSpecificAddressTransferHookTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _RestrictSpecificAddressTransferHook.Contract.contract.Transact(opts, method, params...)
}

// IsTransferRestricted is a free data retrieval call binding the contract method 0xe4e69baf.
//
// Solidity: function isTransferRestricted(address from, address to, (uint256,string) amount) pure returns(bool)
func (_RestrictSpecificAddressTransferHook *RestrictSpecificAddressTransferHookCaller) IsTransferRestricted(opts *bind.CallOpts, from common.Address, to common.Address, amount CosmosCoin) (bool, error) {
	var out []interface{}
	err := _RestrictSpecificAddressTransferHook.contract.Call(opts, &out, "isTransferRestricted", from, to, amount)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsTransferRestricted is a free data retrieval call binding the contract method 0xe4e69baf.
//
// Solidity: function isTransferRestricted(address from, address to, (uint256,string) amount) pure returns(bool)
func (_RestrictSpecificAddressTransferHook *RestrictSpecificAddressTransferHookSession) IsTransferRestricted(from common.Address, to common.Address, amount CosmosCoin) (bool, error) {
	return _RestrictSpecificAddressTransferHook.Contract.IsTransferRestricted(&_RestrictSpecificAddressTransferHook.CallOpts, from, to, amount)
}

// IsTransferRestricted is a free data retrieval call binding the contract method 0xe4e69baf.
//
// Solidity: function isTransferRestricted(address from, address to, (uint256,string) amount) pure returns(bool)
func (_RestrictSpecificAddressTransferHook *RestrictSpecificAddressTransferHookCallerSession) IsTransferRestricted(from common.Address, to common.Address, amount CosmosCoin) (bool, error) {
	return _RestrictSpecificAddressTransferHook.Contract.IsTransferRestricted(&_RestrictSpecificAddressTransferHook.CallOpts, from, to, amount)
}
