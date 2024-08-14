package seth_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/seth"
	network_debug_contract "github.com/smartcontractkit/seth/contracts/bind/debug"
	"github.com/smartcontractkit/seth/test_utils"
)

func commonEnvVars(t *testing.T) {
	t.Setenv(seth.NETWORK_ENV_VAR, seth.GETH)
	t.Setenv(seth.ROOT_PRIVATE_KEY_ENV_VAR, "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	t.Setenv(seth.CONFIG_FILE_ENV_VAR, "../seth.toml")
}

func deployDebugContracts(t *testing.T) *network_debug_contract.NetworkDebugContract {
	c, err := seth.NewClient()
	require.NoError(t, err, "failed to initalise seth")
	nonce := c.NonceManager.NextNonce(c.Addresses[0])
	require.NoError(t, err, "failed to initalise seth")
	subData, err := c.DeployContractFromContractStore(c.NewTXOpts(), "NetworkDebugSubContract.abi")
	require.NoError(t, err, "failed to deploy sub-debug contract")
	data, err := c.DeployContractFromContractStore(c.NewTXOpts(), "NetworkDebugContract.abi", subData.Address)
	require.NoError(t, err, "failed to deploy debug contract")
	contract, err := network_debug_contract.NewNetworkDebugContract(data.Address, c.Client)
	require.NoError(t, err, "failed to create debug contract instance")
	// these ^ are internal methods, so we need to update nonces manually
	err = c.NonceManager.UpdateNonces()
	require.NoError(t, err)
	nonce2 := c.NonceManager.NextNonce(c.Addresses[0])
	require.Equal(t, big.NewInt(0).Add(nonce, big.NewInt(2)).String(), nonce2.String(), "nonces should be updated after contract deployment")

	return contract
}

func setup(t *testing.T) *network_debug_contract.NetworkDebugContract {
	commonEnvVars(t)
	return deployDebugContracts(t)
}

func TestSmokeExampleWait(t *testing.T) {
	contract := setup(t)
	c, err := seth.NewClient()
	require.NoError(t, err)

	// receive decoded transaction or decoded err in case of revert
	dec, err := c.Decode(
		contract.Set(c.NewTXOpts(), big.NewInt(1)),
	)
	require.NoError(t, err)
	// original data
	_ = dec.Transaction
	_ = dec.Receipt
	// decoded data
	_ = dec.Input
	_ = dec.Output
	_ = dec.Events
	res, err := contract.Get(c.NewCallOpts())
	require.NoError(t, err)
	fmt.Printf("Result: %d", res.Int64())
}

func TestSmokeExampleMultiKey(t *testing.T) {
	// example of using client with multiple keys that are provided in the config
	// in this example we just generate and fund them inside NewClientWithAddresses() function
	// to simulate a case, when they were provided as part of the network config, instead of being
	// generated as ephemeral keys by Seth
	contract := setup(t)
	c := test_utils.NewClientWithAddresses(t, 10)
	t.Cleanup(func() {
		err := seth.ReturnFunds(c, c.Addresses[0].Hex())
		require.NoError(t, err)
	})

	// you can use multiple keys to really execute transactions in parallel
	tx1, err1 := c.Decode(contract.Set(
		c.NewTXKeyOpts(1),
		big.NewInt(1),
	))
	require.NoError(t, err1)
	tx2, err2 := c.Decode(contract.Set(
		c.NewTXKeyOpts(2),
		big.NewInt(1),
	))
	require.NoError(t, err2)
	// original data
	_ = tx1.Transaction
	_ = tx1.Receipt
	// decoded data
	_ = tx1.Input
	_ = tx1.Output
	_ = tx1.Events
	// original data
	_ = tx2.Transaction
	_ = tx2.Receipt
	// decoded data
	_ = tx2.Input
	_ = tx2.Output
	_ = tx2.Events
	res, err := contract.Get(c.NewCallOpts())
	require.NoError(t, err)
	fmt.Printf("Result: %d", res.Int64())
}

func TestSmokeExampleMultiKeyEphemeral(t *testing.T) {
	// example of using ephemeral keys
	// suitable for testing ephemeral networks where network is created every time
	contract := setup(t)
	c, err := seth.NewClient()
	require.NoError(t, err)

	// you can use multiple keys to really execute transactions in parallel
	tx1, err1 := c.Decode(contract.Set(
		c.NewTXKeyOpts(1),
		big.NewInt(1),
	))
	require.NoError(t, err1)
	tx2, err2 := c.Decode(contract.Set(
		c.NewTXKeyOpts(2),
		big.NewInt(1),
	))
	require.NoError(t, err2)
	// original data
	_ = tx1.Transaction
	_ = tx1.Receipt
	// decoded data
	_ = tx1.Input
	_ = tx1.Output
	_ = tx1.Events
	// original data
	_ = tx2.Transaction
	_ = tx2.Receipt
	// decoded data
	_ = tx2.Input
	_ = tx2.Output
	_ = tx2.Events
	res, err := contract.Get(c.NewCallOpts())
	require.NoError(t, err)
	fmt.Printf("Result: %d", res.Int64())
}
