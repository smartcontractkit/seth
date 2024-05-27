package seth_test

import (
	"os"
	"testing"

	"github.com/smartcontractkit/seth"
	sethcmd "github.com/smartcontractkit/seth/cmd"
	"github.com/stretchr/testify/require"
)

func TestNetworkFromEnv(t *testing.T) {
	err := os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.CHAIN_ID_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "-n", "Geth", "keys", "split", "-a", "10", "-b", "10"})
	require.NoError(t, err, "Error splitting keys")
}

func TestDefaultNetwork(t *testing.T) {
	err := os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "-c", "1337", "-u", "http://localhost:8545", "keys", "split", "-a", "10", "-b", "10"})
	require.NoError(t, err, "Error splitting keys")
}

func TestDefaultNetworkNoUrl(t *testing.T) {
	err := os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.CHAIN_ID_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "-c", "1337", "keys", "split", "-a", "10", "-b", "10"})
	require.Error(t, err, "No error when splitting keys without URL")
}

func TestDefaultNetworkNoChainID(t *testing.T) {
	err := os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.CHAIN_ID_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "-u", "http://localhost:8545", "keys", "split", "-a", "10", "-b", "10"})
	require.Error(t, err, "No error when splitting keys without URL")
}

func TestDefaultNetworkNoChainIDNoUrlNorNetworkName(t *testing.T) {
	err := os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.CHAIN_ID_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")

	err = sethcmd.RunCLI([]string{"seth", "keys", "split", "-a", "10", "-b", "10"})
	require.Error(t, err, "No error when splitting keys without URL and chain ID")
}
