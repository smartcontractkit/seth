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
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "-n", network, "keys", "fund", "-a", "10", "-b", "10", "--local"})
	require.NoError(t, err, "Error splitting keys")
}

func TestCLIDefaultNetwork(t *testing.T) {
	url, err := getUrlFromEnv()
	require.NoError(t, err, "Error getting url from env")
	network := os.Getenv(seth.NETWORK_ENV_VAR)
	defer func() {
		_ = os.Setenv(seth.NETWORK_ENV_VAR, network)
	}()
	err = os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "-u", url, "keys", "fund", "-a", "10", "-b", "10", "--local"})
	require.NoError(t, err, "Error splitting keys")
}

func TestCLIDefaultNetworkNoUrl(t *testing.T) {
	network := os.Getenv(seth.NETWORK_ENV_VAR)
	defer func() {
		_ = os.Setenv(seth.NETWORK_ENV_VAR, network)
	}()
	err := os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = sethcmd.RunCLI([]string{"seth", "keys", "fund", "-a", "10", "-b", "10", "--local"})
	require.Error(t, err, "No error when splitting keys without url")
}

func TestCLITestDefaultNetworkNoUrlNorNetworkName(t *testing.T) {
	network := os.Getenv(seth.NETWORK_ENV_VAR)
	defer func() {
		_ = os.Setenv(seth.NETWORK_ENV_VAR, network)
	}()
	err := os.Unsetenv(seth.NETWORK_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")
	err = os.Unsetenv(seth.URL_ENV_VAR)
	require.NoError(t, err, "Error unsetting env var")

	err = sethcmd.RunCLI([]string{"seth", "keys", "fund", "-a", "10", "-b", "10", "--local"})
	require.Error(t, err, "No error when splitting keys without url or network name")
}

func getUrlFromEnv() (url string, err error) {
	network := os.Getenv(seth.NETWORK_ENV_VAR)
	switch network {
	case "Geth":
		url = "ws://localhost:8546"
	case "Anvil":
		url = "ws://localhost:8545"
	default:
		err = fmt.Errorf("unsupported network : %s", network)
	}
	return
}
