package seth_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/smartcontractkit/seth"
	sethcmd "github.com/smartcontractkit/seth/cmd"
	"github.com/stretchr/testify/require"
)

func TestCLINetworkFromEnv(t *testing.T) {
	network := os.Getenv(seth.NETWORK_ENV_VAR)
	defer func() {
		_ = os.Setenv(seth.NETWORK_ENV_VAR, network)
	}()
	err := os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.CHAIN_ID_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "-n", network, "keys", "split", "-a", "10", "-b", "10"})
	require.NoError(t, err, "Error splitting keys")
}

func TestCLIDefaultNetwork(t *testing.T) {
	url, chainId, err := getUrlAndChainIdFromEnv()
	require.NoError(t, err, "Error getting URL and chain ID from env")
	network := os.Getenv(seth.NETWORK_ENV_VAR)
	defer func() {
		_ = os.Setenv(seth.NETWORK_ENV_VAR, network)
	}()
	err = os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "-c", chainId, "-u", url, "keys", "split", "-a", "10", "-b", "10"})
	require.NoError(t, err, "Error splitting keys")
}

func TestCLIDefaultNetworkNoUrl(t *testing.T) {
	_, chainId, err := getUrlAndChainIdFromEnv()
	require.NoError(t, err, "Error getting URL and chain ID from env")
	network := os.Getenv(seth.NETWORK_ENV_VAR)
	defer func() {
		_ = os.Setenv(seth.NETWORK_ENV_VAR, network)
	}()
	err = os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.CHAIN_ID_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "-c", chainId, "keys", "split", "-a", "10", "-b", "10"})
	require.Error(t, err, "No error when splitting keys without URL")
}

func TestCLITestDefaultNetworkNoChainID(t *testing.T) {
	url, _, err := getUrlAndChainIdFromEnv()
	require.NoError(t, err, "Error getting URL and chain ID from env")
	network := os.Getenv(seth.NETWORK_ENV_VAR)
	defer func() {
		_ = os.Setenv(seth.NETWORK_ENV_VAR, network)
	}()
	err = os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.CHAIN_ID_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "-u", url, "keys", "split", "-a", "10", "-b", "10"})
	require.Error(t, err, "No error when splitting keys without URL")
}

func TestCLITestDefaultNetworkNoChainIDNoUrlNorNetworkName(t *testing.T) {
	err := os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	network := os.Getenv(seth.NETWORK_ENV_VAR)
	defer func() {
		_ = os.Setenv(seth.NETWORK_ENV_VAR, network)
	}()
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.CHAIN_ID_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")

	err = sethcmd.RunCLI([]string{"seth", "keys", "split", "-a", "10", "-b", "10"})
	require.Error(t, err, "No error when splitting keys without URL and chain ID")
}

func getUrlAndChainIdFromEnv() (url, chainId string, err error) {
	network := os.Getenv(seth.NETWORK_ENV_VAR)
	switch network {
	case "Geth":
		url = "ws://localhost:8546"
		chainId = "1337"
	case "Anvil":
		url = "ws://localhost:8545"
		chainId = "31337"
	default:
		err = fmt.Errorf("unsupported network : %s", network)
	}
	return
}
