package seth_test

import (
	"fmt"
	"math/big"
	"os"
	"testing"

	"github.com/smartcontractkit/seth"
	sethcmd "github.com/smartcontractkit/seth/cmd"
	network_debug_contract "github.com/smartcontractkit/seth/contracts/bind/debug"
	"github.com/stretchr/testify/require"
)

func commonEnvVars(t *testing.T) {
	t.Setenv("NETWORK", seth.GETH)
	t.Setenv("ROOT_PRIVATE_KEY", "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	t.Setenv("SETH_CONFIG_PATH", "../seth.toml")
}

func deployDebugContracts(t *testing.T) *network_debug_contract.NetworkDebugContract {
	c, err := seth.NewClient()
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

func commonMultiKeySetup(t *testing.T) {
	_ = os.Remove("keyfile_test_example.toml")
	t.Setenv("SETH_KEYFILE_PATH", "keyfile_test_example.toml")
	err := sethcmd.RunCLI([]string{"seth", "-n", os.Getenv("NETWORK"), "keys", "split", "-a", "2"})
	require.NoError(t, err)
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
	// example of using a static keyfile with multiple keys
	// see commonMultiKeySetup for env vars
	contract := setup(t)
	commonMultiKeySetup(t)
	c, err := seth.NewClient()
	require.NoError(t, err)
	t.Cleanup(func() {
		err = sethcmd.RunCLI([]string{"seth", "-n", os.Getenv("NETWORK"), "keys", "return"})
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
	// suitable for testing simulated networks where network is created every time
	// ephemeral mode is used if SETH_KEYFILE_PATH is empty
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
