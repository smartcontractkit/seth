package seth_test

import (
	"github.com/smartcontractkit/seth"
	"math/big"
	"os"
	"testing"

	sethcmd "github.com/smartcontractkit/seth/cmd"
	"github.com/stretchr/testify/require"
)

func AssertFileBalances(t *testing.T, amount *big.Int) {
	c := newClient(t)
	kf, err := c.CreateOrUnmarshalKeyFile(nil)
	require.NoError(t, err)
	for _, kfd := range kf.Keys {
		require.NotEmpty(t, kfd.PrivateKey)
		require.NotEmpty(t, kfd.Address)
		require.NotEmpty(t, kfd.Funds)
		if amount != nil {
			require.Equal(t, amount.String(), kfd.Funds)
		}
	}
}

func TestCLIFundAndReturn(t *testing.T) {
	_ = os.Remove("keyfile_test.toml")
	c := newClient(t)
	for i := 0; i < 3; i++ {
		bd, err := c.CalculateSubKeyFunding(10)
		require.NoError(t, err)
		err = sethcmd.RunCLI([]string{"seth", "-n", os.Getenv("NETWORK"), "keys", "split", "-a", "10"})
		require.NoError(t, err)
		AssertFileBalances(t, bd.AddrFunding)
		err = sethcmd.RunCLI([]string{"seth", "-n", os.Getenv("NETWORK"), "keys", "return"})
		require.NoError(t, err)
		// TODO: since estimation logic is dynamic and not complete yet we should assert it properly later
	}
}

func TestCLIUpdateBalances(t *testing.T) {
	_ = os.Remove("keyfile_test_2.toml")
	_ = os.Setenv("SETH_KEYFILE_PATH", "keyfile_test_2.toml")
	err := sethcmd.RunCLI([]string{"seth", "-n", os.Getenv("NETWORK"), "keys", "split", "-a", "2"})
	require.NoError(t, err)
	c := newClient(t)
	_, err = c.Decode(
		TestEnv.DebugContract.Pay(
			c.NewTXKeyOpts(2, seth.WithValue(big.NewInt(1e9))),
		),
	)
	require.NoError(t, err)
	err = sethcmd.RunCLI([]string{"seth", "-n", os.Getenv("NETWORK"), "keys", "update"})
	require.NoError(t, err)
	kf, err := c.CreateOrUnmarshalKeyFile(nil)
	require.NoError(t, err)
	require.NotEqual(t, kf.Keys[0].Funds, kf.Keys[1].Funds)
	err = sethcmd.RunCLI([]string{"seth", "-n", os.Getenv("NETWORK"), "keys", "return"})
	require.NoError(t, err)
}
