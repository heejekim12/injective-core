// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package exchange

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

// ExchangeTestMetaData contains all meta data concerning the ExchangeTest contract.
var ExchangeTestMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"counter\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"delegateDeposit\",\"inputs\":[{\"name\":\"subaccountID\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"denom\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"deposit\",\"inputs\":[{\"name\":\"subaccountID\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"denom\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"depositAndRevert\",\"inputs\":[{\"name\":\"subaccountID\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"denom\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"depositTest1\",\"inputs\":[{\"name\":\"subaccountID\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"denom\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"depositTest2\",\"inputs\":[{\"name\":\"subaccountID\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"denom\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"withdraw\",\"inputs\":[{\"name\":\"subaccountID\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"denom\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"}]",
	Bin: "0x60806040525f80546001600160a01b03191660651790553480156020575f5ffd5b506106c98061002e5f395ff3fe608060405234801561000f575f5ffd5b506004361061007a575f3560e01c806361bc221a1161005857806361bc221a146100cc578063ba73b818146100e3578063e653ab54146100f6578063f81a095f14610109575f5ffd5b8063222ac0551461007e5780634864c6ab146100a657806354c25b6d146100b9575b5f5ffd5b61009161008c3660046104e2565b61011c565b60405190151581526020015b60405180910390f35b6100916100b43660046104e2565b61019c565b6100916100c73660046104e2565b61023e565b6100d560015481565b60405190815260200161009d565b6100916100f13660046104e2565b6102e8565b6100916101043660046104e2565b61031e565b6100916101173660046104e2565b610396565b5f805460405163e441dec960e01b81526001600160a01b039091169063e441dec99061015290309088908890889060040161059c565b6020604051808303815f875af115801561016e573d5f5f3e3d5ffd5b505050506040513d601f19601f8201168201806040525081019061019291906105e3565b90505b9392505050565b5f5f60656001600160a01b0316308686866040516024016101c0949392919061059c565b60408051601f198184030181529181526020820180516001600160e01b031663b24bd3f960e01b179052516101f59190610602565b5f60405180830381855af49150503d805f811461022d576040519150601f19603f3d011682016040523d82523d5f602084013e610232565b606091505b50909695505050505050565b600180545f918261024e83610631565b90915550505f805460405163e441dec960e01b81526001600160a01b039091169063e441dec99061028990339089908990899060040161059c565b6020604051808303815f875af11580156102a5573d5f5f3e3d5ffd5b505050506040513d601f19601f820116820180604052508101906102c991906105e3565b600180549192505f6102da83610649565b909155509095945050505050565b5f805460405163eb28c20560e01b81526001600160a01b039091169063eb28c2059061015290309088908890889060040161059c565b60405163f81a095f60e01b81525f90309063f81a095f906103479087908790879060040161065e565b6020604051808303815f875af1925050508015610381575060408051601f3d908101601f1916820190925261037e918101906105e3565b60015b1561038d579050610195565b505f9392505050565b5f805460405163e441dec960e01b81526001600160a01b039091169063e441dec9906103cc90339088908890889060040161059c565b6020604051808303815f875af11580156103e8573d5f5f3e3d5ffd5b505050506040513d601f19601f8201168201806040525081019061040c91906105e3565b5060405162461bcd60e51b815260206004820152600760248201526674657374696e6760c81b604482015260640160405180910390fd5b634e487b7160e01b5f52604160045260245ffd5b5f82601f830112610466575f5ffd5b813567ffffffffffffffff81111561048057610480610443565b604051601f8201601f19908116603f0116810167ffffffffffffffff811182821017156104af576104af610443565b6040528181528382016020018510156104c6575f5ffd5b816020850160208301375f918101602001919091529392505050565b5f5f5f606084860312156104f4575f5ffd5b833567ffffffffffffffff81111561050a575f5ffd5b61051686828701610457565b935050602084013567ffffffffffffffff811115610532575f5ffd5b61053e86828701610457565b925050604084013590509250925092565b5f5b83811015610569578181015183820152602001610551565b50505f910152565b5f815180845261058881602086016020860161054f565b601f01601f19169290920160200192915050565b6001600160a01b03851681526080602082018190525f906105bf90830186610571565b82810360408401526105d18186610571565b91505082606083015295945050505050565b5f602082840312156105f3575f5ffd5b81518015158114610195575f5ffd5b5f825161061381846020870161054f565b9190910192915050565b634e487b7160e01b5f52601160045260245ffd5b5f600182016106425761064261061d565b5060010190565b5f816106575761065761061d565b505f190190565b606081525f6106706060830186610571565b82810360208401526106828186610571565b91505082604083015294935050505056fea26469706673582212206dff4cacc15b75c8adbc8591847be7be44a2c3015912f8e3c5cc2f269d0ba23164736f6c634300081e0033",
}

// ExchangeTestABI is the input ABI used to generate the binding from.
// Deprecated: Use ExchangeTestMetaData.ABI instead.
var ExchangeTestABI = ExchangeTestMetaData.ABI

// ExchangeTestBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use ExchangeTestMetaData.Bin instead.
var ExchangeTestBin = ExchangeTestMetaData.Bin

// DeployExchangeTest deploys a new Ethereum contract, binding an instance of ExchangeTest to it.
func DeployExchangeTest(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *ExchangeTest, error) {
	parsed, err := ExchangeTestMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(ExchangeTestBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &ExchangeTest{ExchangeTestCaller: ExchangeTestCaller{contract: contract}, ExchangeTestTransactor: ExchangeTestTransactor{contract: contract}, ExchangeTestFilterer: ExchangeTestFilterer{contract: contract}}, nil
}

// ExchangeTest is an auto generated Go binding around an Ethereum contract.
type ExchangeTest struct {
	ExchangeTestCaller     // Read-only binding to the contract
	ExchangeTestTransactor // Write-only binding to the contract
	ExchangeTestFilterer   // Log filterer for contract events
}

// ExchangeTestCaller is an auto generated read-only Go binding around an Ethereum contract.
type ExchangeTestCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ExchangeTestTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ExchangeTestTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ExchangeTestFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ExchangeTestFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ExchangeTestSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ExchangeTestSession struct {
	Contract     *ExchangeTest     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ExchangeTestCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ExchangeTestCallerSession struct {
	Contract *ExchangeTestCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// ExchangeTestTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ExchangeTestTransactorSession struct {
	Contract     *ExchangeTestTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// ExchangeTestRaw is an auto generated low-level Go binding around an Ethereum contract.
type ExchangeTestRaw struct {
	Contract *ExchangeTest // Generic contract binding to access the raw methods on
}

// ExchangeTestCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ExchangeTestCallerRaw struct {
	Contract *ExchangeTestCaller // Generic read-only contract binding to access the raw methods on
}

// ExchangeTestTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ExchangeTestTransactorRaw struct {
	Contract *ExchangeTestTransactor // Generic write-only contract binding to access the raw methods on
}

// NewExchangeTest creates a new instance of ExchangeTest, bound to a specific deployed contract.
func NewExchangeTest(address common.Address, backend bind.ContractBackend) (*ExchangeTest, error) {
	contract, err := bindExchangeTest(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ExchangeTest{ExchangeTestCaller: ExchangeTestCaller{contract: contract}, ExchangeTestTransactor: ExchangeTestTransactor{contract: contract}, ExchangeTestFilterer: ExchangeTestFilterer{contract: contract}}, nil
}

// NewExchangeTestCaller creates a new read-only instance of ExchangeTest, bound to a specific deployed contract.
func NewExchangeTestCaller(address common.Address, caller bind.ContractCaller) (*ExchangeTestCaller, error) {
	contract, err := bindExchangeTest(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ExchangeTestCaller{contract: contract}, nil
}

// NewExchangeTestTransactor creates a new write-only instance of ExchangeTest, bound to a specific deployed contract.
func NewExchangeTestTransactor(address common.Address, transactor bind.ContractTransactor) (*ExchangeTestTransactor, error) {
	contract, err := bindExchangeTest(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ExchangeTestTransactor{contract: contract}, nil
}

// NewExchangeTestFilterer creates a new log filterer instance of ExchangeTest, bound to a specific deployed contract.
func NewExchangeTestFilterer(address common.Address, filterer bind.ContractFilterer) (*ExchangeTestFilterer, error) {
	contract, err := bindExchangeTest(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ExchangeTestFilterer{contract: contract}, nil
}

// bindExchangeTest binds a generic wrapper to an already deployed contract.
func bindExchangeTest(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ExchangeTestMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ExchangeTest *ExchangeTestRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ExchangeTest.Contract.ExchangeTestCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ExchangeTest *ExchangeTestRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ExchangeTest.Contract.ExchangeTestTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ExchangeTest *ExchangeTestRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ExchangeTest.Contract.ExchangeTestTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ExchangeTest *ExchangeTestCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ExchangeTest.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ExchangeTest *ExchangeTestTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ExchangeTest.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ExchangeTest *ExchangeTestTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ExchangeTest.Contract.contract.Transact(opts, method, params...)
}

// Counter is a free data retrieval call binding the contract method 0x61bc221a.
//
// Solidity: function counter() view returns(uint256)
func (_ExchangeTest *ExchangeTestCaller) Counter(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _ExchangeTest.contract.Call(opts, &out, "counter")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Counter is a free data retrieval call binding the contract method 0x61bc221a.
//
// Solidity: function counter() view returns(uint256)
func (_ExchangeTest *ExchangeTestSession) Counter() (*big.Int, error) {
	return _ExchangeTest.Contract.Counter(&_ExchangeTest.CallOpts)
}

// Counter is a free data retrieval call binding the contract method 0x61bc221a.
//
// Solidity: function counter() view returns(uint256)
func (_ExchangeTest *ExchangeTestCallerSession) Counter() (*big.Int, error) {
	return _ExchangeTest.Contract.Counter(&_ExchangeTest.CallOpts)
}

// DelegateDeposit is a paid mutator transaction binding the contract method 0x4864c6ab.
//
// Solidity: function delegateDeposit(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactor) DelegateDeposit(opts *bind.TransactOpts, subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.contract.Transact(opts, "delegateDeposit", subaccountID, denom, amount)
}

// DelegateDeposit is a paid mutator transaction binding the contract method 0x4864c6ab.
//
// Solidity: function delegateDeposit(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestSession) DelegateDeposit(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.DelegateDeposit(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// DelegateDeposit is a paid mutator transaction binding the contract method 0x4864c6ab.
//
// Solidity: function delegateDeposit(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactorSession) DelegateDeposit(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.DelegateDeposit(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// Deposit is a paid mutator transaction binding the contract method 0x222ac055.
//
// Solidity: function deposit(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactor) Deposit(opts *bind.TransactOpts, subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.contract.Transact(opts, "deposit", subaccountID, denom, amount)
}

// Deposit is a paid mutator transaction binding the contract method 0x222ac055.
//
// Solidity: function deposit(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestSession) Deposit(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.Deposit(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// Deposit is a paid mutator transaction binding the contract method 0x222ac055.
//
// Solidity: function deposit(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactorSession) Deposit(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.Deposit(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// DepositAndRevert is a paid mutator transaction binding the contract method 0xf81a095f.
//
// Solidity: function depositAndRevert(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactor) DepositAndRevert(opts *bind.TransactOpts, subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.contract.Transact(opts, "depositAndRevert", subaccountID, denom, amount)
}

// DepositAndRevert is a paid mutator transaction binding the contract method 0xf81a095f.
//
// Solidity: function depositAndRevert(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestSession) DepositAndRevert(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.DepositAndRevert(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// DepositAndRevert is a paid mutator transaction binding the contract method 0xf81a095f.
//
// Solidity: function depositAndRevert(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactorSession) DepositAndRevert(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.DepositAndRevert(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// DepositTest1 is a paid mutator transaction binding the contract method 0x54c25b6d.
//
// Solidity: function depositTest1(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactor) DepositTest1(opts *bind.TransactOpts, subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.contract.Transact(opts, "depositTest1", subaccountID, denom, amount)
}

// DepositTest1 is a paid mutator transaction binding the contract method 0x54c25b6d.
//
// Solidity: function depositTest1(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestSession) DepositTest1(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.DepositTest1(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// DepositTest1 is a paid mutator transaction binding the contract method 0x54c25b6d.
//
// Solidity: function depositTest1(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactorSession) DepositTest1(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.DepositTest1(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// DepositTest2 is a paid mutator transaction binding the contract method 0xe653ab54.
//
// Solidity: function depositTest2(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactor) DepositTest2(opts *bind.TransactOpts, subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.contract.Transact(opts, "depositTest2", subaccountID, denom, amount)
}

// DepositTest2 is a paid mutator transaction binding the contract method 0xe653ab54.
//
// Solidity: function depositTest2(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestSession) DepositTest2(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.DepositTest2(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// DepositTest2 is a paid mutator transaction binding the contract method 0xe653ab54.
//
// Solidity: function depositTest2(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactorSession) DepositTest2(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.DepositTest2(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// Withdraw is a paid mutator transaction binding the contract method 0xba73b818.
//
// Solidity: function withdraw(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactor) Withdraw(opts *bind.TransactOpts, subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.contract.Transact(opts, "withdraw", subaccountID, denom, amount)
}

// Withdraw is a paid mutator transaction binding the contract method 0xba73b818.
//
// Solidity: function withdraw(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestSession) Withdraw(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.Withdraw(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}

// Withdraw is a paid mutator transaction binding the contract method 0xba73b818.
//
// Solidity: function withdraw(string subaccountID, string denom, uint256 amount) returns(bool)
func (_ExchangeTest *ExchangeTestTransactorSession) Withdraw(subaccountID string, denom string, amount *big.Int) (*types.Transaction, error) {
	return _ExchangeTest.Contract.Withdraw(&_ExchangeTest.TransactOpts, subaccountID, denom, amount)
}
