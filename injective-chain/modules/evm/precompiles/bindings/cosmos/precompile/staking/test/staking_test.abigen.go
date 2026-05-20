// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package staking

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

// StakingTestMetaData contains all meta data concerning the StakingTest contract.
var StakingTestMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"delegate\",\"inputs\":[{\"name\":\"validatorAddress\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"delegation\",\"inputs\":[{\"name\":\"delegatorAddress\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"validatorAddress\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"shares\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"balance\",\"type\":\"tuple\",\"internalType\":\"structCosmos.Coin\",\"components\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"denom\",\"type\":\"string\",\"internalType\":\"string\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"redelegate\",\"inputs\":[{\"name\":\"validatorSrc\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"validatorDst\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"undelegate\",\"inputs\":[{\"name\":\"validatorAddress\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"withdrawDelegatorRewards\",\"inputs\":[{\"name\":\"validatorAddress\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"amount\",\"type\":\"tuple[]\",\"internalType\":\"structCosmos.Coin[]\",\"components\":[{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"denom\",\"type\":\"string\",\"internalType\":\"string\"}]}],\"stateMutability\":\"nonpayable\"}]",
	Bin: "0x60806040525f80546001600160a01b03191660661790553480156020575f5ffd5b506108848061002e5f395ff3fe608060405234801561000f575f5ffd5b5060043610610055575f3560e01c806303f24de114610059578063241774e6146100815780636636125e146100a25780637dd0209d146100c25780638dfc8897146100d5575b5f5ffd5b61006c6100673660046103dd565b6100e8565b60405190151581526020015b60405180910390f35b61009461008f36600461041e565b610161565b6040516100789291906104e0565b6100b56100b03660046104f8565b6101fb565b6040516100789190610529565b61006c6100d036600461058c565b610273565b61006c6100e33660046103dd565b6102ef565b5f80546040516303f24de160e01b81526001600160a01b03909116906303f24de19061011a90869086906004016105f8565b6020604051808303815f875af1158015610136573d5f5f3e3d5ffd5b505050506040513d601f19601f8201168201806040525081019061015a9190610619565b9392505050565b5f61017e60405180604001604052805f8152602001606081525090565b5f5460405163120bba7360e11b81526001600160a01b039091169063241774e6906101af9087908790600401610638565b5f60405180830381865afa1580156101c9573d5f5f3e3d5ffd5b505050506040513d5f823e601f3d908101601f191682016040526101f09190810190610704565b915091509250929050565b5f5460405163331b092f60e11b81526060916001600160a01b031690636636125e9061022b90859060040161073e565b5f604051808303815f875af1158015610246573d5f5f3e3d5ffd5b505050506040513d5f823e601f3d908101601f1916820160405261026d9190810190610750565b92915050565b5f8054604051637dd0209d60e01b81526001600160a01b0390911690637dd0209d906102a790879087908790600401610819565b6020604051808303815f875af11580156102c3573d5f5f3e3d5ffd5b505050506040513d601f19601f820116820180604052508101906102e79190610619565b949350505050565b5f8054604051638dfc889760e01b81526001600160a01b0390911690638dfc88979061011a90869086906004016105f8565b634e487b7160e01b5f52604160045260245ffd5b604051601f8201601f191681016001600160401b038111828210171561035d5761035d610321565b604052919050565b5f6001600160401b0382111561037d5761037d610321565b50601f01601f191660200190565b5f82601f83011261039a575f5ffd5b81356103ad6103a882610365565b610335565b8181528460208386010111156103c1575f5ffd5b816020850160208301375f918101602001919091529392505050565b5f5f604083850312156103ee575f5ffd5b82356001600160401b03811115610403575f5ffd5b61040f8582860161038b565b95602094909401359450505050565b5f5f6040838503121561042f575f5ffd5b82356001600160a01b0381168114610445575f5ffd5b915060208301356001600160401b0381111561045f575f5ffd5b61046b8582860161038b565b9150509250929050565b5f5b8381101561048f578181015183820152602001610477565b50505f910152565b5f81518084526104ae816020860160208601610475565b601f01601f19169290920160200192915050565b805182525f6020820151604060208501526102e76040850182610497565b828152604060208201525f6102e760408301846104c2565b5f60208284031215610508575f5ffd5b81356001600160401b0381111561051d575f5ffd5b6102e78482850161038b565b5f602082016020835280845180835260408501915060408160051b8601019250602086015f5b8281101561058057603f1987860301845261056b8583516104c2565b9450602093840193919091019060010161054f565b50929695505050505050565b5f5f5f6060848603121561059e575f5ffd5b83356001600160401b038111156105b3575f5ffd5b6105bf8682870161038b565b93505060208401356001600160401b038111156105da575f5ffd5b6105e68682870161038b565b93969395505050506040919091013590565b604081525f61060a6040830185610497565b90508260208301529392505050565b5f60208284031215610629575f5ffd5b8151801515811461015a575f5ffd5b6001600160a01b03831681526040602082018190525f906102e790830184610497565b5f6040828403121561066b575f5ffd5b604080519081016001600160401b038111828210171561068d5761068d610321565b60405282518152602083015190915081906001600160401b038111156106b1575f5ffd5b8301601f810185136106c1575f5ffd5b80516106cf6103a882610365565b8181528660208385010111156106e3575f5ffd5b6106f4826020830160208601610475565b8060208501525050505092915050565b5f5f60408385031215610715575f5ffd5b825160208401519092506001600160401b03811115610732575f5ffd5b61046b8582860161065b565b602081525f61015a6020830184610497565b5f60208284031215610760575f5ffd5b81516001600160401b03811115610775575f5ffd5b8201601f81018413610785575f5ffd5b80516001600160401b0381111561079e5761079e610321565b8060051b6107ae60208201610335565b918252602081840181019290810190878411156107c9575f5ffd5b6020850192505b8383101561080e5782516001600160401b038111156107ed575f5ffd5b6107fc8960208389010161065b565b835250602092830192909101906107d0565b979650505050505050565b606081525f61082b6060830186610497565b828103602084015261083d8186610497565b91505082604083015294935050505056fea264697066735822122020dd81d151fd0e02a6b29572baadcfbaba330355265307d900657ad08c07c9fc64736f6c634300081e0033",
}

// StakingTestABI is the input ABI used to generate the binding from.
// Deprecated: Use StakingTestMetaData.ABI instead.
var StakingTestABI = StakingTestMetaData.ABI

// StakingTestBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use StakingTestMetaData.Bin instead.
var StakingTestBin = StakingTestMetaData.Bin

// DeployStakingTest deploys a new Ethereum contract, binding an instance of StakingTest to it.
func DeployStakingTest(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *StakingTest, error) {
	parsed, err := StakingTestMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(StakingTestBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &StakingTest{StakingTestCaller: StakingTestCaller{contract: contract}, StakingTestTransactor: StakingTestTransactor{contract: contract}, StakingTestFilterer: StakingTestFilterer{contract: contract}}, nil
}

// StakingTest is an auto generated Go binding around an Ethereum contract.
type StakingTest struct {
	StakingTestCaller     // Read-only binding to the contract
	StakingTestTransactor // Write-only binding to the contract
	StakingTestFilterer   // Log filterer for contract events
}

// StakingTestCaller is an auto generated read-only Go binding around an Ethereum contract.
type StakingTestCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// StakingTestTransactor is an auto generated write-only Go binding around an Ethereum contract.
type StakingTestTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// StakingTestFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type StakingTestFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// StakingTestSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type StakingTestSession struct {
	Contract     *StakingTest      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// StakingTestCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type StakingTestCallerSession struct {
	Contract *StakingTestCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// StakingTestTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type StakingTestTransactorSession struct {
	Contract     *StakingTestTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// StakingTestRaw is an auto generated low-level Go binding around an Ethereum contract.
type StakingTestRaw struct {
	Contract *StakingTest // Generic contract binding to access the raw methods on
}

// StakingTestCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type StakingTestCallerRaw struct {
	Contract *StakingTestCaller // Generic read-only contract binding to access the raw methods on
}

// StakingTestTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type StakingTestTransactorRaw struct {
	Contract *StakingTestTransactor // Generic write-only contract binding to access the raw methods on
}

// NewStakingTest creates a new instance of StakingTest, bound to a specific deployed contract.
func NewStakingTest(address common.Address, backend bind.ContractBackend) (*StakingTest, error) {
	contract, err := bindStakingTest(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &StakingTest{StakingTestCaller: StakingTestCaller{contract: contract}, StakingTestTransactor: StakingTestTransactor{contract: contract}, StakingTestFilterer: StakingTestFilterer{contract: contract}}, nil
}

// NewStakingTestCaller creates a new read-only instance of StakingTest, bound to a specific deployed contract.
func NewStakingTestCaller(address common.Address, caller bind.ContractCaller) (*StakingTestCaller, error) {
	contract, err := bindStakingTest(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &StakingTestCaller{contract: contract}, nil
}

// NewStakingTestTransactor creates a new write-only instance of StakingTest, bound to a specific deployed contract.
func NewStakingTestTransactor(address common.Address, transactor bind.ContractTransactor) (*StakingTestTransactor, error) {
	contract, err := bindStakingTest(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &StakingTestTransactor{contract: contract}, nil
}

// NewStakingTestFilterer creates a new log filterer instance of StakingTest, bound to a specific deployed contract.
func NewStakingTestFilterer(address common.Address, filterer bind.ContractFilterer) (*StakingTestFilterer, error) {
	contract, err := bindStakingTest(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &StakingTestFilterer{contract: contract}, nil
}

// bindStakingTest binds a generic wrapper to an already deployed contract.
func bindStakingTest(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := StakingTestMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_StakingTest *StakingTestRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _StakingTest.Contract.StakingTestCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_StakingTest *StakingTestRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _StakingTest.Contract.StakingTestTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_StakingTest *StakingTestRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _StakingTest.Contract.StakingTestTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_StakingTest *StakingTestCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _StakingTest.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_StakingTest *StakingTestTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _StakingTest.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_StakingTest *StakingTestTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _StakingTest.Contract.contract.Transact(opts, method, params...)
}

// Delegation is a free data retrieval call binding the contract method 0x241774e6.
//
// Solidity: function delegation(address delegatorAddress, string validatorAddress) view returns(uint256 shares, (uint256,string) balance)
func (_StakingTest *StakingTestCaller) Delegation(opts *bind.CallOpts, delegatorAddress common.Address, validatorAddress string) (struct {
	Shares  *big.Int
	Balance CosmosCoin
}, error) {
	var out []interface{}
	err := _StakingTest.contract.Call(opts, &out, "delegation", delegatorAddress, validatorAddress)

	outstruct := new(struct {
		Shares  *big.Int
		Balance CosmosCoin
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Shares = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.Balance = *abi.ConvertType(out[1], new(CosmosCoin)).(*CosmosCoin)

	return *outstruct, err

}

// Delegation is a free data retrieval call binding the contract method 0x241774e6.
//
// Solidity: function delegation(address delegatorAddress, string validatorAddress) view returns(uint256 shares, (uint256,string) balance)
func (_StakingTest *StakingTestSession) Delegation(delegatorAddress common.Address, validatorAddress string) (struct {
	Shares  *big.Int
	Balance CosmosCoin
}, error) {
	return _StakingTest.Contract.Delegation(&_StakingTest.CallOpts, delegatorAddress, validatorAddress)
}

// Delegation is a free data retrieval call binding the contract method 0x241774e6.
//
// Solidity: function delegation(address delegatorAddress, string validatorAddress) view returns(uint256 shares, (uint256,string) balance)
func (_StakingTest *StakingTestCallerSession) Delegation(delegatorAddress common.Address, validatorAddress string) (struct {
	Shares  *big.Int
	Balance CosmosCoin
}, error) {
	return _StakingTest.Contract.Delegation(&_StakingTest.CallOpts, delegatorAddress, validatorAddress)
}

// Delegate is a paid mutator transaction binding the contract method 0x03f24de1.
//
// Solidity: function delegate(string validatorAddress, uint256 amount) returns(bool)
func (_StakingTest *StakingTestTransactor) Delegate(opts *bind.TransactOpts, validatorAddress string, amount *big.Int) (*types.Transaction, error) {
	return _StakingTest.contract.Transact(opts, "delegate", validatorAddress, amount)
}

// Delegate is a paid mutator transaction binding the contract method 0x03f24de1.
//
// Solidity: function delegate(string validatorAddress, uint256 amount) returns(bool)
func (_StakingTest *StakingTestSession) Delegate(validatorAddress string, amount *big.Int) (*types.Transaction, error) {
	return _StakingTest.Contract.Delegate(&_StakingTest.TransactOpts, validatorAddress, amount)
}

// Delegate is a paid mutator transaction binding the contract method 0x03f24de1.
//
// Solidity: function delegate(string validatorAddress, uint256 amount) returns(bool)
func (_StakingTest *StakingTestTransactorSession) Delegate(validatorAddress string, amount *big.Int) (*types.Transaction, error) {
	return _StakingTest.Contract.Delegate(&_StakingTest.TransactOpts, validatorAddress, amount)
}

// Redelegate is a paid mutator transaction binding the contract method 0x7dd0209d.
//
// Solidity: function redelegate(string validatorSrc, string validatorDst, uint256 amount) returns(bool)
func (_StakingTest *StakingTestTransactor) Redelegate(opts *bind.TransactOpts, validatorSrc string, validatorDst string, amount *big.Int) (*types.Transaction, error) {
	return _StakingTest.contract.Transact(opts, "redelegate", validatorSrc, validatorDst, amount)
}

// Redelegate is a paid mutator transaction binding the contract method 0x7dd0209d.
//
// Solidity: function redelegate(string validatorSrc, string validatorDst, uint256 amount) returns(bool)
func (_StakingTest *StakingTestSession) Redelegate(validatorSrc string, validatorDst string, amount *big.Int) (*types.Transaction, error) {
	return _StakingTest.Contract.Redelegate(&_StakingTest.TransactOpts, validatorSrc, validatorDst, amount)
}

// Redelegate is a paid mutator transaction binding the contract method 0x7dd0209d.
//
// Solidity: function redelegate(string validatorSrc, string validatorDst, uint256 amount) returns(bool)
func (_StakingTest *StakingTestTransactorSession) Redelegate(validatorSrc string, validatorDst string, amount *big.Int) (*types.Transaction, error) {
	return _StakingTest.Contract.Redelegate(&_StakingTest.TransactOpts, validatorSrc, validatorDst, amount)
}

// Undelegate is a paid mutator transaction binding the contract method 0x8dfc8897.
//
// Solidity: function undelegate(string validatorAddress, uint256 amount) returns(bool)
func (_StakingTest *StakingTestTransactor) Undelegate(opts *bind.TransactOpts, validatorAddress string, amount *big.Int) (*types.Transaction, error) {
	return _StakingTest.contract.Transact(opts, "undelegate", validatorAddress, amount)
}

// Undelegate is a paid mutator transaction binding the contract method 0x8dfc8897.
//
// Solidity: function undelegate(string validatorAddress, uint256 amount) returns(bool)
func (_StakingTest *StakingTestSession) Undelegate(validatorAddress string, amount *big.Int) (*types.Transaction, error) {
	return _StakingTest.Contract.Undelegate(&_StakingTest.TransactOpts, validatorAddress, amount)
}

// Undelegate is a paid mutator transaction binding the contract method 0x8dfc8897.
//
// Solidity: function undelegate(string validatorAddress, uint256 amount) returns(bool)
func (_StakingTest *StakingTestTransactorSession) Undelegate(validatorAddress string, amount *big.Int) (*types.Transaction, error) {
	return _StakingTest.Contract.Undelegate(&_StakingTest.TransactOpts, validatorAddress, amount)
}

// WithdrawDelegatorRewards is a paid mutator transaction binding the contract method 0x6636125e.
//
// Solidity: function withdrawDelegatorRewards(string validatorAddress) returns((uint256,string)[] amount)
func (_StakingTest *StakingTestTransactor) WithdrawDelegatorRewards(opts *bind.TransactOpts, validatorAddress string) (*types.Transaction, error) {
	return _StakingTest.contract.Transact(opts, "withdrawDelegatorRewards", validatorAddress)
}

// WithdrawDelegatorRewards is a paid mutator transaction binding the contract method 0x6636125e.
//
// Solidity: function withdrawDelegatorRewards(string validatorAddress) returns((uint256,string)[] amount)
func (_StakingTest *StakingTestSession) WithdrawDelegatorRewards(validatorAddress string) (*types.Transaction, error) {
	return _StakingTest.Contract.WithdrawDelegatorRewards(&_StakingTest.TransactOpts, validatorAddress)
}

// WithdrawDelegatorRewards is a paid mutator transaction binding the contract method 0x6636125e.
//
// Solidity: function withdrawDelegatorRewards(string validatorAddress) returns((uint256,string)[] amount)
func (_StakingTest *StakingTestTransactorSession) WithdrawDelegatorRewards(validatorAddress string) (*types.Transaction, error) {
	return _StakingTest.Contract.WithdrawDelegatorRewards(&_StakingTest.TransactOpts, validatorAddress)
}
