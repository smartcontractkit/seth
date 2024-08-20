package test_utils

import (
	"context"
	"math/big"
	"testing"

	"github.com/smartcontractkit/seth"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// NewClientWithAddresses creates a new Seth client with the given number of addresses. Each address is funded with the
// calculated with the amount of ETH calculated by dividing the total balance of root key by the number of addresses (minus root key buffer amount).
func NewClientWithAddresses(t *testing.T, addressCount int) *seth.Client {
	cfg, err := seth.ReadConfig()
	require.NoError(t, err, "failed to read config")

	var zero int64 = 0
	cfg.EphemeralAddrs = &zero

	c, err := seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to initialize seth")

	var privateKeys []string
	var addresses []string
	for i := 0; i < addressCount; i++ {
		addr, pk, err := seth.NewAddress()
		require.NoError(t, err, "failed to generate new address")

		privateKeys = append(privateKeys, pk)
		addresses = append(addresses, addr)
	}

	gasPrice, err := c.GetSuggestedLegacyFees(context.Background(), seth.Priority_Standard)
	if err != nil {
		gasPrice = big.NewInt(c.Cfg.Network.GasPrice)
	}

	bd, err := c.CalculateSubKeyFunding(int64(addressCount), gasPrice.Int64(), *cfg.RootKeyFundsBuffer)
	require.NoError(t, err, "failed to calculate subkey funding")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eg, egCtx := errgroup.WithContext(ctx)
	// root key is element 0 in ephemeral
	for _, addr := range addresses {
		addr := addr
		eg.Go(func() error {
			return c.TransferETHFromKey(egCtx, 0, addr, bd.AddrFunding, gasPrice)
		})
	}
	err = eg.Wait()
	require.NoError(t, err, "failed to transfer funds to subkeys")

	// Add root private key to the list of private keys
	pksToUse := []string{cfg.Network.PrivateKeys[0]}
	pksToUse = append(pksToUse, privateKeys...)
	// Set funded private keys in config and create a new Seth client to simulate a situation, in which PKs were passed in config to a new client
	cfg.Network.PrivateKeys = pksToUse

	newClient, err := seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to initialize new Seth with private keys")

	return newClient
}
