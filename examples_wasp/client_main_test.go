package examples_wasp

import (
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/smartcontractkit/seth"
	ndbc "github.com/smartcontractkit/seth/contracts/bind/debug"
	nsdbc "github.com/smartcontractkit/seth/contracts/bind/sub"
)

func init() {
	_ = os.Setenv("SETH_CONFIG_PATH", "seth.toml")
}

var (
	TestEnv TestEnvironment
)

type TestEnvironment struct {
	Client                  *seth.Client
	DebugContract           *ndbc.NetworkDebugContract
	DebugSubContract        *nsdbc.NetworkDebugSubContract
	DebugContractAddress    common.Address
	DebugSubContractAddress common.Address
	DebugContractRaw        *bind.BoundContract
	ContractMap             map[string]string
}

func NewDebugContractSetup() (
	*seth.Client,
	*ndbc.NetworkDebugContract,
	common.Address,
	common.Address,
	*bind.BoundContract,
	error,
) {
	cfg, err := seth.ReadConfig()
	if err != nil {
		return nil, nil, common.Address{}, common.Address{}, nil, err
	}
	cs, err := seth.NewContractStore("./contracts/abi", "./contracts/bin")
	if err != nil {
		return nil, nil, common.Address{}, common.Address{}, nil, err
	}
	addrs, pkeys, err := cfg.ParseKeys()
	if err != nil {
		return nil, nil, common.Address{}, common.Address{}, nil, err
	}
	contractMap := make(map[string]string)

	abiFinder := seth.NewABIFinder(contractMap, cs)
	tracer, err := seth.NewTracer(cfg.Network.URLs[0], cs, &abiFinder, cfg, contractMap, addrs)
	if err != nil {
		return nil, nil, common.Address{}, common.Address{}, nil, err
	}

	c, err := seth.NewClientRaw(cfg, addrs, pkeys, seth.WithContractStore(cs), seth.WithTracer(tracer))
	if err != nil {
		return nil, nil, common.Address{}, common.Address{}, nil, err
	}
	subData, err := c.DeployContractFromContractStore(c.NewTXOpts(), "NetworkDebugSubContract.abi")
	if err != nil {
		return nil, nil, common.Address{}, common.Address{}, nil, err
	}
	data, err := c.DeployContractFromContractStore(c.NewTXOpts(), "NetworkDebugContract.abi", subData.Address)
	if err != nil {
		return nil, nil, common.Address{}, common.Address{}, nil, err
	}
	contract, err := ndbc.NewNetworkDebugContract(data.Address, c.Client)
	if err != nil {
		return nil, nil, common.Address{}, common.Address{}, nil, err
	}
	return c, contract, data.Address, subData.Address, data.BoundContract, nil
}

func TestMain(m *testing.M) {
	var err error
	client, debugContract, debugContractAddress, debugSubContractAddress, debugContractRaw, err := NewDebugContractSetup()
	if err != nil {
		panic(err)
	}

	contractMap := make(map[string]string)
	for k, v := range client.ContractAddressToNameMap {
		contractMap[k] = v
	}

	TestEnv = TestEnvironment{
		Client:                  client,
		DebugContract:           debugContract,
		DebugContractAddress:    debugContractAddress,
		DebugSubContractAddress: debugSubContractAddress,
		DebugContractRaw:        debugContractRaw,
		ContractMap:             contractMap,
	}

	exitVal := m.Run()
	os.Exit(exitVal)
}
