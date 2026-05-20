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

// OracleTestMetaData contains all meta data concerning the OracleTest contract.
var OracleTestMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"oraclePrice\",\"inputs\":[{\"name\":\"oracleType\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"base\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"quote\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"oraclePricePairState\",\"inputs\":[{\"name\":\"oracleType\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"base\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"quote\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structIOracleModule.PricePairState\",\"components\":[{\"name\":\"pairPrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"basePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quotePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quoteCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"quoteTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"oraclePricePairStateScaled\",\"inputs\":[{\"name\":\"oracleType\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"base\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"quote\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"baseDecimals\",\"type\":\"uint32\",\"internalType\":\"uint32\"},{\"name\":\"quoteDecimals\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structIOracleModule.PricePairState\",\"components\":[{\"name\":\"pairPrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"basePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quotePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quoteCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"quoteTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"oraclePricePairStateScaledViaStaticCall\",\"inputs\":[{\"name\":\"oracleType\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"base\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"quote\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"baseDecimals\",\"type\":\"uint32\",\"internalType\":\"uint32\"},{\"name\":\"quoteDecimals\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"outputs\":[{\"name\":\"state\",\"type\":\"tuple\",\"internalType\":\"structIOracleModule.PricePairState\",\"components\":[{\"name\":\"pairPrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"basePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quotePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quoteCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"quoteTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"oraclePricePairStateViaStaticCall\",\"inputs\":[{\"name\":\"oracleType\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"base\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"quote\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"state\",\"type\":\"tuple\",\"internalType\":\"structIOracleModule.PricePairState\",\"components\":[{\"name\":\"pairPrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"basePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quotePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"quoteCumulativePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"baseTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"quoteTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"oraclePriceViaStaticCall\",\"inputs\":[{\"name\":\"oracleType\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"base\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"quote\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"price\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"}]",
	Bin: "0x60806040525f80546001600160a01b03191660671790553480156020575f5ffd5b506109c88061002e5f395ff3fe608060405234801561000f575f5ffd5b5060043610610060575f3560e01c806320374400146100645780635d98e604146100e95780636a06eb6f146100fc578063858255441461011d578063c24bfb3214610130578063e7912d5f14610143575b5f5ffd5b6100776100723660046106a9565b610156565b6040516100e091905f60e082019050825182526020830151602083015260408301516040830152606083015160608301526080830151608083015267ffffffffffffffff60a08401511660a083015267ffffffffffffffff60c08401511660c083015292915050565b60405180910390f35b6100776100f7366004610730565b6101d7565b61010f61010a3660046106a9565b6102da565b6040519081526020016100e0565b61007761012b3660046106a9565b61034d565b61007761013e366004610730565b610441565b61010f6101513660046106a9565b6104c9565b61015e6105ab565b5f5460405162080dd160ea1b81526001600160a01b039091169063203744009061019090879087908790600401610812565b60e060405180830381865afa1580156101ab573d5f5f3e3d5ffd5b505050506040513d601f19601f820116820180604052508101906101cf9190610856565b949350505050565b6101df6105ab565b5f63c24bfb3260e01b87878787876040516024016102019594939291906108e3565b604051602081830303815290604052906001600160e01b0319166020820180516001600160e01b03838183161783525050505090505f5f60676001600160a01b0316836040516102519190610935565b5f60405180830381855afa9150503d805f8114610289576040519150601f19603f3d011682016040523d82523d5f602084013e61028e565b606091505b5091509150816102b95760405162461bcd60e51b81526004016102b090610950565b60405180910390fd5b808060200190518101906102cd9190610856565b9998505050505050505050565b5f8054604051636a06eb6f60e01b81526001600160a01b0390911690636a06eb6f9061030e90879087908790600401610812565b602060405180830381865afa158015610329573d5f5f3e3d5ffd5b505050506040513d601f19601f820116820180604052508101906101cf919061097b565b6103556105ab565b5f632037440060e01b85858560405160240161037393929190610812565b604051602081830303815290604052906001600160e01b0319166020820180516001600160e01b03838183161783525050505090505f5f60676001600160a01b0316836040516103c39190610935565b5f60405180830381855afa9150503d805f81146103fb576040519150601f19603f3d011682016040523d82523d5f602084013e610400565b606091505b5091509150816104225760405162461bcd60e51b81526004016102b090610950565b808060200190518101906104369190610856565b979650505050505050565b6104496105ab565b5f54604051636125fd9960e11b81526001600160a01b039091169063c24bfb329061048090899089908990899089906004016108e3565b60e060405180830381865afa15801561049b573d5f5f3e3d5ffd5b505050506040513d601f19601f820116820180604052508101906104bf9190610856565b9695505050505050565b5f5f636a06eb6f60e01b8585856040516024016104e893929190610812565b604051602081830303815290604052906001600160e01b0319166020820180516001600160e01b03838183161783525050505090505f5f60676001600160a01b0316836040516105389190610935565b5f60405180830381855afa9150503d805f8114610570576040519150601f19603f3d011682016040523d82523d5f602084013e610575565b606091505b5091509150816105975760405162461bcd60e51b81526004016102b090610950565b80806020019051810190610436919061097b565b6040518060e001604052805f81526020015f81526020015f81526020015f81526020015f81526020015f67ffffffffffffffff1681526020015f67ffffffffffffffff1681525090565b803560ff81168114610605575f5ffd5b919050565b634e487b7160e01b5f52604160045260245ffd5b5f82601f83011261062d575f5ffd5b813567ffffffffffffffff8111156106475761064761060a565b604051601f8201601f19908116603f0116810167ffffffffffffffff811182821017156106765761067661060a565b60405281815283820160200185101561068d575f5ffd5b816020850160208301375f918101602001919091529392505050565b5f5f5f606084860312156106bb575f5ffd5b6106c4846105f5565b9250602084013567ffffffffffffffff8111156106df575f5ffd5b6106eb8682870161061e565b925050604084013567ffffffffffffffff811115610707575f5ffd5b6107138682870161061e565b9150509250925092565b803563ffffffff81168114610605575f5ffd5b5f5f5f5f5f60a08688031215610744575f5ffd5b61074d866105f5565b9450602086013567ffffffffffffffff811115610768575f5ffd5b6107748882890161061e565b945050604086013567ffffffffffffffff811115610790575f5ffd5b61079c8882890161061e565b9350506107ab6060870161071d565b91506107b96080870161071d565b90509295509295909350565b5f5b838110156107df5781810151838201526020016107c7565b50505f910152565b5f81518084526107fe8160208601602086016107c5565b601f01601f19169290920160200192915050565b60ff84168152606060208201525f61082d60608301856107e7565b82810360408401526104bf81856107e7565b805167ffffffffffffffff81168114610605575f5ffd5b5f60e0828403128015610867575f5ffd5b5060405160e0810167ffffffffffffffff8111828210171561088b5761088b61060a565b60409081528351825260208085015190830152838101519082015260608084015190820152608080840151908201526108c660a0840161083f565b60a08201526108d760c0840161083f565b60c08201529392505050565b60ff8616815260a060208201525f6108fe60a08301876107e7565b828103604084015261091081876107e7565b91505063ffffffff8416606083015263ffffffff831660808301529695505050505050565b5f82516109468184602087016107c5565b9190910192915050565b6020808252601190820152701cdd185d1a58d8d85b1b0819985a5b1959607a1b604082015260600190565b5f6020828403121561098b575f5ffd5b505191905056fea2646970667358221220d9c982547025f5011fbd5e568d515e37788de6b08d0b6a214870a8d9211fea6964736f6c634300081e0033",
}

// OracleTestABI is the input ABI used to generate the binding from.
// Deprecated: Use OracleTestMetaData.ABI instead.
var OracleTestABI = OracleTestMetaData.ABI

// OracleTestBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use OracleTestMetaData.Bin instead.
var OracleTestBin = OracleTestMetaData.Bin

// DeployOracleTest deploys a new Ethereum contract, binding an instance of OracleTest to it.
func DeployOracleTest(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *OracleTest, error) {
	parsed, err := OracleTestMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(OracleTestBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &OracleTest{OracleTestCaller: OracleTestCaller{contract: contract}, OracleTestTransactor: OracleTestTransactor{contract: contract}, OracleTestFilterer: OracleTestFilterer{contract: contract}}, nil
}

// OracleTest is an auto generated Go binding around an Ethereum contract.
type OracleTest struct {
	OracleTestCaller     // Read-only binding to the contract
	OracleTestTransactor // Write-only binding to the contract
	OracleTestFilterer   // Log filterer for contract events
}

// OracleTestCaller is an auto generated read-only Go binding around an Ethereum contract.
type OracleTestCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OracleTestTransactor is an auto generated write-only Go binding around an Ethereum contract.
type OracleTestTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OracleTestFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type OracleTestFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OracleTestSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type OracleTestSession struct {
	Contract     *OracleTest       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// OracleTestCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type OracleTestCallerSession struct {
	Contract *OracleTestCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// OracleTestTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type OracleTestTransactorSession struct {
	Contract     *OracleTestTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// OracleTestRaw is an auto generated low-level Go binding around an Ethereum contract.
type OracleTestRaw struct {
	Contract *OracleTest // Generic contract binding to access the raw methods on
}

// OracleTestCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type OracleTestCallerRaw struct {
	Contract *OracleTestCaller // Generic read-only contract binding to access the raw methods on
}

// OracleTestTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type OracleTestTransactorRaw struct {
	Contract *OracleTestTransactor // Generic write-only contract binding to access the raw methods on
}

// NewOracleTest creates a new instance of OracleTest, bound to a specific deployed contract.
func NewOracleTest(address common.Address, backend bind.ContractBackend) (*OracleTest, error) {
	contract, err := bindOracleTest(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &OracleTest{OracleTestCaller: OracleTestCaller{contract: contract}, OracleTestTransactor: OracleTestTransactor{contract: contract}, OracleTestFilterer: OracleTestFilterer{contract: contract}}, nil
}

// NewOracleTestCaller creates a new read-only instance of OracleTest, bound to a specific deployed contract.
func NewOracleTestCaller(address common.Address, caller bind.ContractCaller) (*OracleTestCaller, error) {
	contract, err := bindOracleTest(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &OracleTestCaller{contract: contract}, nil
}

// NewOracleTestTransactor creates a new write-only instance of OracleTest, bound to a specific deployed contract.
func NewOracleTestTransactor(address common.Address, transactor bind.ContractTransactor) (*OracleTestTransactor, error) {
	contract, err := bindOracleTest(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &OracleTestTransactor{contract: contract}, nil
}

// NewOracleTestFilterer creates a new log filterer instance of OracleTest, bound to a specific deployed contract.
func NewOracleTestFilterer(address common.Address, filterer bind.ContractFilterer) (*OracleTestFilterer, error) {
	contract, err := bindOracleTest(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &OracleTestFilterer{contract: contract}, nil
}

// bindOracleTest binds a generic wrapper to an already deployed contract.
func bindOracleTest(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := OracleTestMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_OracleTest *OracleTestRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _OracleTest.Contract.OracleTestCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_OracleTest *OracleTestRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _OracleTest.Contract.OracleTestTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_OracleTest *OracleTestRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _OracleTest.Contract.OracleTestTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_OracleTest *OracleTestCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _OracleTest.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_OracleTest *OracleTestTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _OracleTest.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_OracleTest *OracleTestTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _OracleTest.Contract.contract.Transact(opts, method, params...)
}

// OraclePrice is a free data retrieval call binding the contract method 0x6a06eb6f.
//
// Solidity: function oraclePrice(uint8 oracleType, string base, string quote) view returns(uint256)
func (_OracleTest *OracleTestCaller) OraclePrice(opts *bind.CallOpts, oracleType uint8, base string, quote string) (*big.Int, error) {
	var out []interface{}
	err := _OracleTest.contract.Call(opts, &out, "oraclePrice", oracleType, base, quote)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// OraclePrice is a free data retrieval call binding the contract method 0x6a06eb6f.
//
// Solidity: function oraclePrice(uint8 oracleType, string base, string quote) view returns(uint256)
func (_OracleTest *OracleTestSession) OraclePrice(oracleType uint8, base string, quote string) (*big.Int, error) {
	return _OracleTest.Contract.OraclePrice(&_OracleTest.CallOpts, oracleType, base, quote)
}

// OraclePrice is a free data retrieval call binding the contract method 0x6a06eb6f.
//
// Solidity: function oraclePrice(uint8 oracleType, string base, string quote) view returns(uint256)
func (_OracleTest *OracleTestCallerSession) OraclePrice(oracleType uint8, base string, quote string) (*big.Int, error) {
	return _OracleTest.Contract.OraclePrice(&_OracleTest.CallOpts, oracleType, base, quote)
}

// OraclePricePairState is a free data retrieval call binding the contract method 0x20374400.
//
// Solidity: function oraclePricePairState(uint8 oracleType, string base, string quote) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64))
func (_OracleTest *OracleTestCaller) OraclePricePairState(opts *bind.CallOpts, oracleType uint8, base string, quote string) (IOracleModulePricePairState, error) {
	var out []interface{}
	err := _OracleTest.contract.Call(opts, &out, "oraclePricePairState", oracleType, base, quote)

	if err != nil {
		return *new(IOracleModulePricePairState), err
	}

	out0 := *abi.ConvertType(out[0], new(IOracleModulePricePairState)).(*IOracleModulePricePairState)

	return out0, err

}

// OraclePricePairState is a free data retrieval call binding the contract method 0x20374400.
//
// Solidity: function oraclePricePairState(uint8 oracleType, string base, string quote) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64))
func (_OracleTest *OracleTestSession) OraclePricePairState(oracleType uint8, base string, quote string) (IOracleModulePricePairState, error) {
	return _OracleTest.Contract.OraclePricePairState(&_OracleTest.CallOpts, oracleType, base, quote)
}

// OraclePricePairState is a free data retrieval call binding the contract method 0x20374400.
//
// Solidity: function oraclePricePairState(uint8 oracleType, string base, string quote) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64))
func (_OracleTest *OracleTestCallerSession) OraclePricePairState(oracleType uint8, base string, quote string) (IOracleModulePricePairState, error) {
	return _OracleTest.Contract.OraclePricePairState(&_OracleTest.CallOpts, oracleType, base, quote)
}

// OraclePricePairStateScaled is a free data retrieval call binding the contract method 0xc24bfb32.
//
// Solidity: function oraclePricePairStateScaled(uint8 oracleType, string base, string quote, uint32 baseDecimals, uint32 quoteDecimals) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64))
func (_OracleTest *OracleTestCaller) OraclePricePairStateScaled(opts *bind.CallOpts, oracleType uint8, base string, quote string, baseDecimals uint32, quoteDecimals uint32) (IOracleModulePricePairState, error) {
	var out []interface{}
	err := _OracleTest.contract.Call(opts, &out, "oraclePricePairStateScaled", oracleType, base, quote, baseDecimals, quoteDecimals)

	if err != nil {
		return *new(IOracleModulePricePairState), err
	}

	out0 := *abi.ConvertType(out[0], new(IOracleModulePricePairState)).(*IOracleModulePricePairState)

	return out0, err

}

// OraclePricePairStateScaled is a free data retrieval call binding the contract method 0xc24bfb32.
//
// Solidity: function oraclePricePairStateScaled(uint8 oracleType, string base, string quote, uint32 baseDecimals, uint32 quoteDecimals) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64))
func (_OracleTest *OracleTestSession) OraclePricePairStateScaled(oracleType uint8, base string, quote string, baseDecimals uint32, quoteDecimals uint32) (IOracleModulePricePairState, error) {
	return _OracleTest.Contract.OraclePricePairStateScaled(&_OracleTest.CallOpts, oracleType, base, quote, baseDecimals, quoteDecimals)
}

// OraclePricePairStateScaled is a free data retrieval call binding the contract method 0xc24bfb32.
//
// Solidity: function oraclePricePairStateScaled(uint8 oracleType, string base, string quote, uint32 baseDecimals, uint32 quoteDecimals) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64))
func (_OracleTest *OracleTestCallerSession) OraclePricePairStateScaled(oracleType uint8, base string, quote string, baseDecimals uint32, quoteDecimals uint32) (IOracleModulePricePairState, error) {
	return _OracleTest.Contract.OraclePricePairStateScaled(&_OracleTest.CallOpts, oracleType, base, quote, baseDecimals, quoteDecimals)
}

// OraclePricePairStateScaledViaStaticCall is a free data retrieval call binding the contract method 0x5d98e604.
//
// Solidity: function oraclePricePairStateScaledViaStaticCall(uint8 oracleType, string base, string quote, uint32 baseDecimals, uint32 quoteDecimals) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleTest *OracleTestCaller) OraclePricePairStateScaledViaStaticCall(opts *bind.CallOpts, oracleType uint8, base string, quote string, baseDecimals uint32, quoteDecimals uint32) (IOracleModulePricePairState, error) {
	var out []interface{}
	err := _OracleTest.contract.Call(opts, &out, "oraclePricePairStateScaledViaStaticCall", oracleType, base, quote, baseDecimals, quoteDecimals)

	if err != nil {
		return *new(IOracleModulePricePairState), err
	}

	out0 := *abi.ConvertType(out[0], new(IOracleModulePricePairState)).(*IOracleModulePricePairState)

	return out0, err

}

// OraclePricePairStateScaledViaStaticCall is a free data retrieval call binding the contract method 0x5d98e604.
//
// Solidity: function oraclePricePairStateScaledViaStaticCall(uint8 oracleType, string base, string quote, uint32 baseDecimals, uint32 quoteDecimals) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleTest *OracleTestSession) OraclePricePairStateScaledViaStaticCall(oracleType uint8, base string, quote string, baseDecimals uint32, quoteDecimals uint32) (IOracleModulePricePairState, error) {
	return _OracleTest.Contract.OraclePricePairStateScaledViaStaticCall(&_OracleTest.CallOpts, oracleType, base, quote, baseDecimals, quoteDecimals)
}

// OraclePricePairStateScaledViaStaticCall is a free data retrieval call binding the contract method 0x5d98e604.
//
// Solidity: function oraclePricePairStateScaledViaStaticCall(uint8 oracleType, string base, string quote, uint32 baseDecimals, uint32 quoteDecimals) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleTest *OracleTestCallerSession) OraclePricePairStateScaledViaStaticCall(oracleType uint8, base string, quote string, baseDecimals uint32, quoteDecimals uint32) (IOracleModulePricePairState, error) {
	return _OracleTest.Contract.OraclePricePairStateScaledViaStaticCall(&_OracleTest.CallOpts, oracleType, base, quote, baseDecimals, quoteDecimals)
}

// OraclePricePairStateViaStaticCall is a free data retrieval call binding the contract method 0x85825544.
//
// Solidity: function oraclePricePairStateViaStaticCall(uint8 oracleType, string base, string quote) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleTest *OracleTestCaller) OraclePricePairStateViaStaticCall(opts *bind.CallOpts, oracleType uint8, base string, quote string) (IOracleModulePricePairState, error) {
	var out []interface{}
	err := _OracleTest.contract.Call(opts, &out, "oraclePricePairStateViaStaticCall", oracleType, base, quote)

	if err != nil {
		return *new(IOracleModulePricePairState), err
	}

	out0 := *abi.ConvertType(out[0], new(IOracleModulePricePairState)).(*IOracleModulePricePairState)

	return out0, err

}

// OraclePricePairStateViaStaticCall is a free data retrieval call binding the contract method 0x85825544.
//
// Solidity: function oraclePricePairStateViaStaticCall(uint8 oracleType, string base, string quote) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleTest *OracleTestSession) OraclePricePairStateViaStaticCall(oracleType uint8, base string, quote string) (IOracleModulePricePairState, error) {
	return _OracleTest.Contract.OraclePricePairStateViaStaticCall(&_OracleTest.CallOpts, oracleType, base, quote)
}

// OraclePricePairStateViaStaticCall is a free data retrieval call binding the contract method 0x85825544.
//
// Solidity: function oraclePricePairStateViaStaticCall(uint8 oracleType, string base, string quote) view returns((uint256,uint256,uint256,uint256,uint256,uint64,uint64) state)
func (_OracleTest *OracleTestCallerSession) OraclePricePairStateViaStaticCall(oracleType uint8, base string, quote string) (IOracleModulePricePairState, error) {
	return _OracleTest.Contract.OraclePricePairStateViaStaticCall(&_OracleTest.CallOpts, oracleType, base, quote)
}

// OraclePriceViaStaticCall is a free data retrieval call binding the contract method 0xe7912d5f.
//
// Solidity: function oraclePriceViaStaticCall(uint8 oracleType, string base, string quote) view returns(uint256 price)
func (_OracleTest *OracleTestCaller) OraclePriceViaStaticCall(opts *bind.CallOpts, oracleType uint8, base string, quote string) (*big.Int, error) {
	var out []interface{}
	err := _OracleTest.contract.Call(opts, &out, "oraclePriceViaStaticCall", oracleType, base, quote)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// OraclePriceViaStaticCall is a free data retrieval call binding the contract method 0xe7912d5f.
//
// Solidity: function oraclePriceViaStaticCall(uint8 oracleType, string base, string quote) view returns(uint256 price)
func (_OracleTest *OracleTestSession) OraclePriceViaStaticCall(oracleType uint8, base string, quote string) (*big.Int, error) {
	return _OracleTest.Contract.OraclePriceViaStaticCall(&_OracleTest.CallOpts, oracleType, base, quote)
}

// OraclePriceViaStaticCall is a free data retrieval call binding the contract method 0xe7912d5f.
//
// Solidity: function oraclePriceViaStaticCall(uint8 oracleType, string base, string quote) view returns(uint256 price)
func (_OracleTest *OracleTestCallerSession) OraclePriceViaStaticCall(oracleType uint8, base string, quote string) (*big.Int, error) {
	return _OracleTest.Contract.OraclePriceViaStaticCall(&_OracleTest.CallOpts, oracleType, base, quote)
}
