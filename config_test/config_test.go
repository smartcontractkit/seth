package config_test

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/seth"
	link_token "github.com/smartcontractkit/seth/contracts/bind/link"
)

// We put these tests in a separate package, so that they do not require or use environment variables (e.g. SETH_CONFIG_PATH) that would
// interfere with these tests, because they are read, when new client is created.

func TestDefaultConfig(t *testing.T) {
	cfg := seth.DefaultConfig("ws://localhost:8546", []string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"})
	require.NotNil(t, cfg, "failed to create default config")

	client, err := seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to create client with default config")
	require.Equal(t, 1, len(client.PrivateKeys), "expected 1 private key")

	linkAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get LINK ABI")

	_, err = client.DeployContract(client.NewTXOpts(), "LinkToken", *linkAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.NoError(t, err, "failed to deploy LINK contract")
}

func TestDefaultConfig_TwoPks(t *testing.T) {
	cfg := seth.DefaultConfig("ws://localhost:8546", []string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"})
	require.NotNil(t, cfg, "failed to create default config")

	client, err := seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to create client with default config")
	require.Equal(t, 2, len(client.PrivateKeys), "expected 2 private keys")

	linkAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get LINK ABI")

	_, err = client.DeployContract(client.NewTXOpts(), "LinkToken", *linkAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.NoError(t, err, "failed to deploy LINK contract")
}

func TestDefaultConfig_EmptyUrl(t *testing.T) {
	cfg, err := seth.ValidatedDefaultConfig("", []string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"})
	require.Nil(t, cfg, "expected nil config")
	require.Error(t, err, "succeeded in creating default config")
	require.Equal(t, seth.ErrEmptyRPCURL, err.Error(), "expected empty rpc url error")
}

func TestDefaultConfig_NoPks(t *testing.T) {
	cfg, err := seth.ValidatedDefaultConfig("ws://localhost:8546", []string{})
	require.Nil(t, cfg, "expected nil config")
	require.Error(t, err, "succeeded in creating default config")
	require.Equal(t, seth.ErrNoPrivateKeysPassed, err.Error(), "expected no private keys error")
}

func TestDefaultConfig_InvalidPk(t *testing.T) {
	cfg, err := seth.ValidatedDefaultConfig("ws://localhost:8546", []string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff8"})
	require.Nil(t, cfg, "expected nil config")
	require.Error(t, err, "succeeded in creating default config")
	require.Equal(t, fmt.Sprintf("%s: invalid hex data for private key", seth.ErrInvalidPrivateKey), err.Error(), "expected invalid private key error")
}

func TestDefaultConfig_InvalidAndValidPk(t *testing.T) {
	cfg, err := seth.ValidatedDefaultConfig("ws://localhost:8546", []string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff0", "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"})
	require.Nil(t, cfg, "expected nil config")
	require.Error(t, err, "succeeded in creating default config")
	require.Equal(t, fmt.Sprintf("%s: invalid hex data for private key", seth.ErrInvalidPrivateKey), err.Error(), "expected invalid private key error")
}
