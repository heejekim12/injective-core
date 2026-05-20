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

// IExchangeModuleBatchCreateDerivativeLimitOrdersResponse is an auto generated low-level Go binding around an user-defined struct.
type IExchangeModuleBatchCreateDerivativeLimitOrdersResponse struct {
	OrderHashes       []string
	CreatedOrdersCids []string
	FailedOrdersCids  []string
}

// IExchangeModuleCreateDerivativeLimitOrderResponse is an auto generated low-level Go binding around an user-defined struct.
type IExchangeModuleCreateDerivativeLimitOrderResponse struct {
	OrderHash string
	Cid       string
}

// IExchangeModuleDerivativeOrder is an auto generated low-level Go binding around an user-defined struct.
type IExchangeModuleDerivativeOrder struct {
	MarketID     string
	SubaccountID string
	FeeRecipient string
	Price        *big.Int
	Quantity     *big.Int
	Cid          string
	OrderType    string
	Margin       *big.Int
	TriggerPrice *big.Int
}

// ExchangeProxyMetaData contains all meta data concerning the ExchangeProxy contract.
var ExchangeProxyMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"batchCreateDerivativeLimitOrders\",\"inputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"orders\",\"type\":\"tuple[]\",\"internalType\":\"structIExchangeModule.DerivativeOrder[]\",\"components\":[{\"name\":\"marketID\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"subaccountID\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"feeRecipient\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"price\",\"type\":\"uint256\",\"internalType\":\"ExchangeTypes.UFixed256x18\"},{\"name\":\"quantity\",\"type\":\"uint256\",\"internalType\":\"ExchangeTypes.UFixed256x18\"},{\"name\":\"cid\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"orderType\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"margin\",\"type\":\"uint256\",\"internalType\":\"ExchangeTypes.UFixed256x18\"},{\"name\":\"triggerPrice\",\"type\":\"uint256\",\"internalType\":\"ExchangeTypes.UFixed256x18\"}]}],\"outputs\":[{\"name\":\"response\",\"type\":\"tuple\",\"internalType\":\"structIExchangeModule.BatchCreateDerivativeLimitOrdersResponse\",\"components\":[{\"name\":\"orderHashes\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"createdOrdersCids\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"failedOrdersCids\",\"type\":\"string[]\",\"internalType\":\"string[]\"}]}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"createDerivativeLimitOrder\",\"inputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"order\",\"type\":\"tuple\",\"internalType\":\"structIExchangeModule.DerivativeOrder\",\"components\":[{\"name\":\"marketID\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"subaccountID\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"feeRecipient\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"price\",\"type\":\"uint256\",\"internalType\":\"ExchangeTypes.UFixed256x18\"},{\"name\":\"quantity\",\"type\":\"uint256\",\"internalType\":\"ExchangeTypes.UFixed256x18\"},{\"name\":\"cid\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"orderType\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"margin\",\"type\":\"uint256\",\"internalType\":\"ExchangeTypes.UFixed256x18\"},{\"name\":\"triggerPrice\",\"type\":\"uint256\",\"internalType\":\"ExchangeTypes.UFixed256x18\"}]}],\"outputs\":[{\"name\":\"response\",\"type\":\"tuple\",\"internalType\":\"structIExchangeModule.CreateDerivativeLimitOrderResponse\",\"components\":[{\"name\":\"orderHash\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"cid\",\"type\":\"string\",\"internalType\":\"string\"}]}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"queryAllowance\",\"inputs\":[{\"name\":\"grantee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"granter\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"msgType\",\"type\":\"uint8\",\"internalType\":\"ExchangeTypes.MsgType\"}],\"outputs\":[{\"name\":\"allowed\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"}]",
	Bin: "0x60806040525f80546001600160a01b03191660651790553480156020575f5ffd5b50610afb8061002e5f395ff3fe608060405234801561000f575f5ffd5b506004361061003f575f3560e01c806320c69837146100435780635ce4a7311461006c57806379374eab1461008f575b5f5ffd5b610056610051366004610373565b6100af565b6040516100639190610411565b60405180910390f35b61007f61007a366004610452565b610195565b6040519015158152602001610063565b6100a261009d36600461049b565b610260565b6040516100639190610574565b604080518082018252606080825260208201525f5491516320c6983760e01b815290916001600160a01b0316906320c69837906100f2908690869060040161071f565b5f604051808303815f875af192505050801561012f57506040513d5f823e601f3d908101601f1916820160405261012c919081019061080e565b60015b61018e5760405162461bcd60e51b815260206004820152602560248201527f6572726f72206372656174696e672064657269766174697665206c696d69742060448201526437b93232b960d91b60648201526084015b60405180910390fd5b9392505050565b5f80546040516304f7c31560e31b81526001600160a01b038681166004830152858116602483015260ff85166044830152909116906327be18a890606401602060405180830381865afa92505050801561020c575060408051601f3d908101601f19168201909252610209918101906108bf565b60015b6102585760405162461bcd60e51b815260206004820152601860248201527f6572726f72207175657279696e6720616c6c6f77616e636500000000000000006044820152606401610185565b949350505050565b61028460405180606001604052806060815260200160608152602001606081525090565b5f546040516379374eab60e01b81526001600160a01b03909116906379374eab906102b7908790879087906004016108de565b5f604051808303815f875af19250505080156102f457506040513d5f823e601f3d908101601f191682016040526102f19190810190610a07565b60015b6102585760405162461bcd60e51b815260206004820152602f60248201527f6572726f72206372656174696e672064657269766174697665206c696d69742060448201526e0dee4c8cae4e640d2dc40c4c2e8c6d608b1b6064820152608401610185565b80356001600160a01b038116811461036e575f5ffd5b919050565b5f5f60408385031215610384575f5ffd5b61038d83610358565b915060208301356001600160401b038111156103a7575f5ffd5b830161012081860312156103b9575f5ffd5b809150509250929050565b5f5b838110156103de5781810151838201526020016103c6565b50505f910152565b5f81518084526103fd8160208601602086016103c4565b601f01601f19169290920160200192915050565b602081525f82516040602084015261042c60608401826103e6565b90506020840151601f1984830301604085015261044982826103e6565b95945050505050565b5f5f5f60608486031215610464575f5ffd5b61046d84610358565b925061047b60208501610358565b9150604084013560ff81168114610490575f5ffd5b809150509250925092565b5f5f5f604084860312156104ad575f5ffd5b6104b684610358565b925060208401356001600160401b038111156104d0575f5ffd5b8401601f810186136104e0575f5ffd5b80356001600160401b038111156104f5575f5ffd5b8660208260051b8401011115610509575f5ffd5b939660209190910195509293505050565b5f82825180855260208501945060208160051b830101602085015f5b8381101561056857601f198584030188526105528383516103e6565b6020988901989093509190910190600101610536565b50909695505050505050565b602081525f82516060602084015261058f608084018261051a565b90506020840151601f198483030160408501526105ac828261051a565b9150506040840151601f19848303016060850152610449828261051a565b5f5f8335601e198436030181126105df575f5ffd5b83016020810192503590506001600160401b038111156105fd575f5ffd5b80360382131561060b575f5ffd5b9250929050565b81835281816020850137505f828201602090810191909152601f909101601f19169091010190565b5f61064582836105ca565b610120855261065961012086018284610612565b91505061066960208401846105ca565b858303602087015261067c838284610612565b9250505061068d60408401846105ca565b85830360408701526106a0838284610612565b606086810135908801526080808701359088015292506106c691505060a08401846105ca565b85830360a08701526106d9838284610612565b925050506106ea60c08401846105ca565b85830360c08701526106fd838284610612565b60e0868101359088015261010095860135959096019490945250929392505050565b6001600160a01b03831681526040602082018190525f906102589083018461063a565b634e487b7160e01b5f52604160045260245ffd5b604051606081016001600160401b038111828210171561077857610778610742565b60405290565b604051601f8201601f191681016001600160401b03811182821017156107a6576107a6610742565b604052919050565b5f82601f8301126107bd575f5ffd5b81516001600160401b038111156107d6576107d6610742565b6107e9601f8201601f191660200161077e565b8181528460208386010111156107fd575f5ffd5b6102588260208301602087016103c4565b5f6020828403121561081e575f5ffd5b81516001600160401b03811115610833575f5ffd5b820160408185031215610844575f5ffd5b604080519081016001600160401b038111828210171561086657610866610742565b60405281516001600160401b0381111561087e575f5ffd5b61088a868285016107ae565b82525060208201516001600160401b038111156108a5575f5ffd5b6108b1868285016107ae565b602083015250949350505050565b5f602082840312156108cf575f5ffd5b8151801515811461018e575f5ffd5b6001600160a01b038416815260406020820181905281018290525f6060600584901b83018101908301858361011e1936839003015b8782101561095757868503605f190184528235818112610931575f5ffd5b61093d868b830161063a565b955050602083019250602084019350600182019150610913565b509298975050505050505050565b5f82601f830112610974575f5ffd5b81516001600160401b0381111561098d5761098d610742565b8060051b61099d6020820161077e565b918252602081850181019290810190868411156109b8575f5ffd5b6020860192505b838310156109fd5782516001600160401b038111156109dc575f5ffd5b6109eb886020838a01016107ae565b835250602092830192909101906109bf565b9695505050505050565b5f60208284031215610a17575f5ffd5b81516001600160401b03811115610a2c575f5ffd5b820160608185031215610a3d575f5ffd5b610a45610756565b81516001600160401b03811115610a5a575f5ffd5b610a6686828501610965565b82525060208201516001600160401b03811115610a81575f5ffd5b610a8d86828501610965565b60208301525060408201516001600160401b03811115610aab575f5ffd5b610ab786828501610965565b60408301525094935050505056fea264697066735822122047d73ea16ea83a39b805a242308e52fce12c4c4c24b135e09c7a78b16630614b64736f6c634300081e0033",
}

// ExchangeProxyABI is the input ABI used to generate the binding from.
// Deprecated: Use ExchangeProxyMetaData.ABI instead.
var ExchangeProxyABI = ExchangeProxyMetaData.ABI

// ExchangeProxyBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use ExchangeProxyMetaData.Bin instead.
var ExchangeProxyBin = ExchangeProxyMetaData.Bin

// DeployExchangeProxy deploys a new Ethereum contract, binding an instance of ExchangeProxy to it.
func DeployExchangeProxy(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *ExchangeProxy, error) {
	parsed, err := ExchangeProxyMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(ExchangeProxyBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &ExchangeProxy{ExchangeProxyCaller: ExchangeProxyCaller{contract: contract}, ExchangeProxyTransactor: ExchangeProxyTransactor{contract: contract}, ExchangeProxyFilterer: ExchangeProxyFilterer{contract: contract}}, nil
}

// ExchangeProxy is an auto generated Go binding around an Ethereum contract.
type ExchangeProxy struct {
	ExchangeProxyCaller     // Read-only binding to the contract
	ExchangeProxyTransactor // Write-only binding to the contract
	ExchangeProxyFilterer   // Log filterer for contract events
}

// ExchangeProxyCaller is an auto generated read-only Go binding around an Ethereum contract.
type ExchangeProxyCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ExchangeProxyTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ExchangeProxyTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ExchangeProxyFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ExchangeProxyFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ExchangeProxySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ExchangeProxySession struct {
	Contract     *ExchangeProxy    // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ExchangeProxyCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ExchangeProxyCallerSession struct {
	Contract *ExchangeProxyCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts        // Call options to use throughout this session
}

// ExchangeProxyTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ExchangeProxyTransactorSession struct {
	Contract     *ExchangeProxyTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts        // Transaction auth options to use throughout this session
}

// ExchangeProxyRaw is an auto generated low-level Go binding around an Ethereum contract.
type ExchangeProxyRaw struct {
	Contract *ExchangeProxy // Generic contract binding to access the raw methods on
}

// ExchangeProxyCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ExchangeProxyCallerRaw struct {
	Contract *ExchangeProxyCaller // Generic read-only contract binding to access the raw methods on
}

// ExchangeProxyTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ExchangeProxyTransactorRaw struct {
	Contract *ExchangeProxyTransactor // Generic write-only contract binding to access the raw methods on
}

// NewExchangeProxy creates a new instance of ExchangeProxy, bound to a specific deployed contract.
func NewExchangeProxy(address common.Address, backend bind.ContractBackend) (*ExchangeProxy, error) {
	contract, err := bindExchangeProxy(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ExchangeProxy{ExchangeProxyCaller: ExchangeProxyCaller{contract: contract}, ExchangeProxyTransactor: ExchangeProxyTransactor{contract: contract}, ExchangeProxyFilterer: ExchangeProxyFilterer{contract: contract}}, nil
}

// NewExchangeProxyCaller creates a new read-only instance of ExchangeProxy, bound to a specific deployed contract.
func NewExchangeProxyCaller(address common.Address, caller bind.ContractCaller) (*ExchangeProxyCaller, error) {
	contract, err := bindExchangeProxy(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ExchangeProxyCaller{contract: contract}, nil
}

// NewExchangeProxyTransactor creates a new write-only instance of ExchangeProxy, bound to a specific deployed contract.
func NewExchangeProxyTransactor(address common.Address, transactor bind.ContractTransactor) (*ExchangeProxyTransactor, error) {
	contract, err := bindExchangeProxy(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ExchangeProxyTransactor{contract: contract}, nil
}

// NewExchangeProxyFilterer creates a new log filterer instance of ExchangeProxy, bound to a specific deployed contract.
func NewExchangeProxyFilterer(address common.Address, filterer bind.ContractFilterer) (*ExchangeProxyFilterer, error) {
	contract, err := bindExchangeProxy(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ExchangeProxyFilterer{contract: contract}, nil
}

// bindExchangeProxy binds a generic wrapper to an already deployed contract.
func bindExchangeProxy(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ExchangeProxyMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ExchangeProxy *ExchangeProxyRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ExchangeProxy.Contract.ExchangeProxyCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ExchangeProxy *ExchangeProxyRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ExchangeProxy.Contract.ExchangeProxyTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ExchangeProxy *ExchangeProxyRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ExchangeProxy.Contract.ExchangeProxyTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ExchangeProxy *ExchangeProxyCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ExchangeProxy.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ExchangeProxy *ExchangeProxyTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ExchangeProxy.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ExchangeProxy *ExchangeProxyTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ExchangeProxy.Contract.contract.Transact(opts, method, params...)
}

// QueryAllowance is a free data retrieval call binding the contract method 0x5ce4a731.
//
// Solidity: function queryAllowance(address grantee, address granter, uint8 msgType) view returns(bool allowed)
func (_ExchangeProxy *ExchangeProxyCaller) QueryAllowance(opts *bind.CallOpts, grantee common.Address, granter common.Address, msgType uint8) (bool, error) {
	var out []interface{}
	err := _ExchangeProxy.contract.Call(opts, &out, "queryAllowance", grantee, granter, msgType)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// QueryAllowance is a free data retrieval call binding the contract method 0x5ce4a731.
//
// Solidity: function queryAllowance(address grantee, address granter, uint8 msgType) view returns(bool allowed)
func (_ExchangeProxy *ExchangeProxySession) QueryAllowance(grantee common.Address, granter common.Address, msgType uint8) (bool, error) {
	return _ExchangeProxy.Contract.QueryAllowance(&_ExchangeProxy.CallOpts, grantee, granter, msgType)
}

// QueryAllowance is a free data retrieval call binding the contract method 0x5ce4a731.
//
// Solidity: function queryAllowance(address grantee, address granter, uint8 msgType) view returns(bool allowed)
func (_ExchangeProxy *ExchangeProxyCallerSession) QueryAllowance(grantee common.Address, granter common.Address, msgType uint8) (bool, error) {
	return _ExchangeProxy.Contract.QueryAllowance(&_ExchangeProxy.CallOpts, grantee, granter, msgType)
}

// BatchCreateDerivativeLimitOrders is a paid mutator transaction binding the contract method 0x79374eab.
//
// Solidity: function batchCreateDerivativeLimitOrders(address sender, (string,string,string,uint256,uint256,string,string,uint256,uint256)[] orders) returns((string[],string[],string[]) response)
func (_ExchangeProxy *ExchangeProxyTransactor) BatchCreateDerivativeLimitOrders(opts *bind.TransactOpts, sender common.Address, orders []IExchangeModuleDerivativeOrder) (*types.Transaction, error) {
	return _ExchangeProxy.contract.Transact(opts, "batchCreateDerivativeLimitOrders", sender, orders)
}

// BatchCreateDerivativeLimitOrders is a paid mutator transaction binding the contract method 0x79374eab.
//
// Solidity: function batchCreateDerivativeLimitOrders(address sender, (string,string,string,uint256,uint256,string,string,uint256,uint256)[] orders) returns((string[],string[],string[]) response)
func (_ExchangeProxy *ExchangeProxySession) BatchCreateDerivativeLimitOrders(sender common.Address, orders []IExchangeModuleDerivativeOrder) (*types.Transaction, error) {
	return _ExchangeProxy.Contract.BatchCreateDerivativeLimitOrders(&_ExchangeProxy.TransactOpts, sender, orders)
}

// BatchCreateDerivativeLimitOrders is a paid mutator transaction binding the contract method 0x79374eab.
//
// Solidity: function batchCreateDerivativeLimitOrders(address sender, (string,string,string,uint256,uint256,string,string,uint256,uint256)[] orders) returns((string[],string[],string[]) response)
func (_ExchangeProxy *ExchangeProxyTransactorSession) BatchCreateDerivativeLimitOrders(sender common.Address, orders []IExchangeModuleDerivativeOrder) (*types.Transaction, error) {
	return _ExchangeProxy.Contract.BatchCreateDerivativeLimitOrders(&_ExchangeProxy.TransactOpts, sender, orders)
}

// CreateDerivativeLimitOrder is a paid mutator transaction binding the contract method 0x20c69837.
//
// Solidity: function createDerivativeLimitOrder(address sender, (string,string,string,uint256,uint256,string,string,uint256,uint256) order) returns((string,string) response)
func (_ExchangeProxy *ExchangeProxyTransactor) CreateDerivativeLimitOrder(opts *bind.TransactOpts, sender common.Address, order IExchangeModuleDerivativeOrder) (*types.Transaction, error) {
	return _ExchangeProxy.contract.Transact(opts, "createDerivativeLimitOrder", sender, order)
}

// CreateDerivativeLimitOrder is a paid mutator transaction binding the contract method 0x20c69837.
//
// Solidity: function createDerivativeLimitOrder(address sender, (string,string,string,uint256,uint256,string,string,uint256,uint256) order) returns((string,string) response)
func (_ExchangeProxy *ExchangeProxySession) CreateDerivativeLimitOrder(sender common.Address, order IExchangeModuleDerivativeOrder) (*types.Transaction, error) {
	return _ExchangeProxy.Contract.CreateDerivativeLimitOrder(&_ExchangeProxy.TransactOpts, sender, order)
}

// CreateDerivativeLimitOrder is a paid mutator transaction binding the contract method 0x20c69837.
//
// Solidity: function createDerivativeLimitOrder(address sender, (string,string,string,uint256,uint256,string,string,uint256,uint256) order) returns((string,string) response)
func (_ExchangeProxy *ExchangeProxyTransactorSession) CreateDerivativeLimitOrder(sender common.Address, order IExchangeModuleDerivativeOrder) (*types.Transaction, error) {
	return _ExchangeProxy.Contract.CreateDerivativeLimitOrder(&_ExchangeProxy.TransactOpts, sender, order)
}
