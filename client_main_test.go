package seth_test

import (
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/smartcontractkit/seth"
	network_debug_contract "github.com/smartcontractkit/seth/contracts/bind/debug"
	network_sub_contract "github.com/smartcontractkit/seth/contracts/bind/sub"
	"github.com/stretchr/testify/require"

	link_token "github.com/smartcontractkit/seth/contracts/bind/link"
)

/*
	Some tests should be run on testnets/mainnets, so we are deploying the contract only once,
	for these types of tests it's always a choice between funds/speed of tests

	If you need unique setup, just use NewDebugContractSetup in tests
*/

func init() {
	_ = os.Setenv("SETH_CONFIG_PATH", "seth.toml")
	_ = os.Setenv("SETH_KEYFILE_PATH", "keyfile_test.toml")
}

var (
	TestEnv TestEnvironment
)

type TestEnvironment struct {
	Client                  *seth.Client
	DebugContract           *network_debug_contract.NetworkDebugContract
	DebugSubContract        *network_sub_contract.NetworkDebugSubContract
	DebugContractAddress    common.Address
	DebugSubContractAddress common.Address
	DebugContractRaw        *bind.BoundContract
	ContractMap             map[string]string
}

func newClient(t *testing.T) *seth.Client {
	c, err := seth.NewClient()
	require.NoError(t, err, "failed to initalise seth")

	return c
}

func TestDeploymentLinkTokenFromGethWrapperExample(t *testing.T) {
	c, err := seth.NewClient()
	require.NoError(t, err, "failed to initalise seth")
	abi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")
	contractData, err := c.DeployContract(c.NewTXOpts(), "LinkToken", *abi, []byte(link_token.LinkTokenMetaData.Bin))
	require.NoError(t, err, "failed to deploy link token contract from wrapper's ABI/BIN")

	contract, err := link_token.NewLinkToken(contractData.Address, c.Client)
	require.NoError(t, err, "failed to create debug contract instance")

	_, err = c.Decode(contract.Mint(c.NewTXOpts(), common.Address{}, big.NewInt(1)))
	require.NoError(t, err, "failed to decode transaction")
}

func newClientWithContractMapFromEnv(t *testing.T) *seth.Client {
	c := newClient(t)
	if len(TestEnv.ContractMap) == 0 {
		t.Fatal("contract map is empty")
	}

	// create a copy of the map, so we don't have problem with side effects of modyfing client's map
	// impacting the global, underlaying one
	contractMap := make(map[string]string)
	for k, v := range TestEnv.ContractMap {
		contractMap[k] = v
	}

	c.ContractAddressToNameMap = contractMap

	// now let's recreate the Tracer, so that it has the same contract map
	tracer, err := seth.NewTracer(c.Cfg.Network.URLs[0], c.ContractStore, c.ABIFinder, c.Cfg, contractMap, c.Addresses)
	require.NoError(t, err, "failed to create tracer")

	c.Tracer = tracer
	c.ABIFinder.ContractMap = contractMap

	return c
}

func NewDebugContractSetup() (
	*seth.Client,
	*network_debug_contract.NetworkDebugContract,
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
	contract, err := network_debug_contract.NewNetworkDebugContract(data.Address, c.Client)
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
