package seth_test

import (
	"fmt"
	"github.com/smartcontractkit/seth"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	link_token "github.com/smartcontractkit/seth/contracts/bind/link"
)

func TestConfig_Default(t *testing.T) {
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

func TestConfig_Default_TwoPks(t *testing.T) {
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

func TestConfig_Default_EmptyUrl(t *testing.T) {
	cfg, err := seth.ValidatedDefaultConfig("", []string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"})
	require.Nil(t, cfg, "expected nil config")
	require.Error(t, err, "succeeded in creating default config")
	require.Equal(t, seth.ErrEmptyRPCURL, err.Error(), "expected empty rpc url error")
}

func TestConfig_Default_NoPks(t *testing.T) {
	cfg, err := seth.ValidatedDefaultConfig("ws://localhost:8546", []string{})
	require.Nil(t, cfg, "expected nil config")
	require.Error(t, err, "succeeded in creating default config")
	require.Equal(t, seth.ErrNoPrivateKeysPassed, err.Error(), "expected no private keys error")
}

func TestConfig_Default_InvalidPk(t *testing.T) {
	cfg, err := seth.ValidatedDefaultConfig("ws://localhost:8546", []string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff8"})
	require.Nil(t, cfg, "expected nil config")
	require.Error(t, err, "succeeded in creating default config")
	require.Equal(t, fmt.Sprintf("%s: invalid hex data for private key", seth.ErrInvalidPrivateKey), err.Error(), "expected invalid private key error")
}

func TestConfig_Default_InvalidAndValidPk(t *testing.T) {
	cfg, err := seth.ValidatedDefaultConfig("ws://localhost:8546", []string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff0", "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"})
	require.Nil(t, cfg, "expected nil config")
	require.Error(t, err, "succeeded in creating default config")
	require.Equal(t, fmt.Sprintf("%s: invalid hex data for private key", seth.ErrInvalidPrivateKey), err.Error(), "expected invalid private key error")
}

func TestConfig_MinimalBuilder(t *testing.T) {
	builder := seth.NewConfigBuilder()

	cfg := builder.WithRpcUrl("ws://localhost:8546").
		WithPrivateKeys([]string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"}).
		Build()

	require.NotNil(t, cfg, "failed to build config")

	client, err := seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to create client")
	require.Equal(t, 1, len(client.PrivateKeys), "expected 1 private key")

	linkAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get LINK ABI")

	_, err = client.DeployContract(client.NewTXOpts(), "LinkToken", *linkAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.NoError(t, err, "failed to deploy LINK contract")
}

func TestConfig_MaximalBuilder(t *testing.T) {
	builder := seth.NewConfigBuilder()

	cfg := builder.
		// network
		WithNetworkName("my network").
		WithRpcUrl("ws://localhost:8546").
		WithPrivateKeys([]string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"}).
		WithRpcDialTimeout(10*time.Second).
		WithTransactionTimeout(1*time.Minute).
		// addresses
		WithEphemeralAddresses(10, 10).
		// tracing
		WithTracing(seth.TracingLevel_All, []string{seth.TraceOutput_Console}).
		// protections
		WithProtections(true, true).
		// artifacts folder
		WithArtifactsFolder("some_folder").
		// nonce manager
		WithNonceManager(10, 3, 60, 5).
		Build()

	require.NotNil(t, cfg, "failed to build config")

	client, err := seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to create client")
	require.Equal(t, 11, len(client.PrivateKeys), "expected 11 private keys")

	t.Cleanup(func() {
		err = seth.ReturnFunds(client, client.Addresses[0].Hex())
		require.NoError(t, err, "failed to return funds")
	})

	linkAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get LINK ABI")

	_, err = client.DeployContract(client.NewTXOpts(), "LinkToken", *linkAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.NoError(t, err, "failed to deploy LINK contract")
}

func TestConfig_LegacyGas_No_Estimations(t *testing.T) {
	builder := seth.NewConfigBuilder()

	cfg := builder.
		// network
		WithNetworkName("my network").
		WithRpcUrl("ws://localhost:8546").
		WithPrivateKeys([]string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"}).
		// Gas price and estimations
		WithLegacyGasPrice(710_000_000).
		WithGasPriceEstimations(false, 0, "").
		Build()

	require.NotNil(t, cfg, "failed to build config")

	client, err := seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to create client")
	require.Equal(t, 1, len(client.PrivateKeys), "expected 1 private key")

	linkAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get LINK ABI")

	_, err = client.DeployContract(client.NewTXOpts(), "LinkToken", *linkAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.NoError(t, err, "failed to deploy LINK contract")
}

func TestConfig_Eip1559Gas_With_Estimations(t *testing.T) {
	builder := seth.NewConfigBuilder()

	cfg := builder.
		// network
		WithNetworkName("my network").
		WithRpcUrl("ws://localhost:8546").
		WithPrivateKeys([]string{"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"}).
		// Gas price and estimations
		WithEIP1559DynamicFees(true).
		WithDynamicGasPrices(120_000_000_000, 44_000_000_000).
		WithGasPriceEstimations(false, 10, seth.Priority_Fast).
		Build()

	require.NotNil(t, cfg, "failed to build config")

	client, err := seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to create client")
	require.Equal(t, 1, len(client.PrivateKeys), "expected 1 private key")

	linkAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get LINK ABI")

	_, err = client.DeployContract(client.NewTXOpts(), "LinkToken", *linkAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.NoError(t, err, "failed to deploy LINK contract")
}
