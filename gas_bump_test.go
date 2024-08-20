package seth_test

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/pelletier/go-toml/v2"
	link_token "github.com/smartcontractkit/seth/contracts/bind/link"
	"github.com/smartcontractkit/seth/contracts/bind/link_token_interface"
	"github.com/smartcontractkit/seth/test_utils"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/seth"
)

func TestGasBumping_Contract_Deployment_Legacy_SufficientBumping(t *testing.T) {
	c := newClient(t)

	// Set a low gas price and a short timeout
	c.Cfg.Network.GasPrice = 1
	c.Cfg.Network.TxnTimeout = seth.MustMakeDuration(10 * time.Second)
	c.Cfg.GasBumpRetries = 10

	client, err := seth.NewClientWithConfig(c.Cfg)
	require.NoError(t, err)

	gasBumps := 0

	client.GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
		gasBumps++
		newGasPrice := new(big.Int).Mul(gasPrice, big.NewInt(100))
		// cap max gas price to avoid hitting upper bound
		if newGasPrice.Cmp(big.NewInt(100000000)) >= 0 {
			return big.NewInt(0).Add(newGasPrice, big.NewInt(1))
		}
		return newGasPrice
	}

	contractAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")

	// Send a transaction with a low gas price
	data, err := client.DeployContract(client.NewTXOpts(), "LinkToken", *contractAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.NoError(t, err, "contract wasn't deployed")
	require.GreaterOrEqual(t, gasBumps, 1, "expected at least one gas bump")
	require.Greater(t, data.Transaction.GasPrice().Int64(), 1, "expected gas price to be bumped")
}

func TestGasBumping_Contract_Deployment_Legacy_InsufficientBumping(t *testing.T) {
	c := newClient(t)

	// Set a low gas price and a short timeout
	c.Cfg.Network.GasPrice = 1
	c.Cfg.Network.TxnTimeout = seth.MustMakeDuration(10 * time.Second)
	c.Cfg.GasBumpRetries = 2

	client, err := seth.NewClientWithConfig(c.Cfg)
	require.NoError(t, err)

	gasBumps := 0

	client.GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
		gasBumps++
		return new(big.Int).Add(gasPrice, big.NewInt(1))
	}

	contractAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")

	// Send a transaction with a low gas price
	_, err = client.DeployContract(client.NewTXOpts(), "LinkToken", *contractAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))

	require.Error(t, err, "contract was deployed, but gas bumping shouldn't be sufficient to deploy it")
	require.GreaterOrEqual(t, gasBumps, 1, "expected at least one gas bump")
}

func TestGasBumping_Contract_Deployment_Legacy_FailedBumping(t *testing.T) {
	c := newClient(t)

	// Set a low gas price and a short timeout
	c.Cfg.Network.GasPrice = 1
	c.Cfg.Network.TxnTimeout = seth.MustMakeDuration(10 * time.Second)
	c.Cfg.GasBumpRetries = 2

	client, err := seth.NewClientWithConfig(c.Cfg)
	require.NoError(t, err)

	gasBumps := 0

	// this results in a gas bump that is too high to be accepted
	client.GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
		gasBumps++
		return new(big.Int).Mul(gasPrice, big.NewInt(1000000000000))
	}

	contractAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")

	// Send a transaction with a low gas price and then bump it too high to be accepted
	_, err = client.DeployContract(client.NewTXOpts(), "LinkToken", *contractAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.Error(t, err, "contract was deployed, but gas bumping should be failing")
	require.GreaterOrEqual(t, gasBumps, 1, "expected at least one gas bump")
}

func TestGasBumping_Contract_Deployment_Legacy_BumpingDisabled(t *testing.T) {
	c := newClient(t)

	// Set a low gas price and a short timeout, but disable gas bumping
	c.Cfg.Network.GasPrice = 1
	c.Cfg.Network.TxnTimeout = seth.MustMakeDuration(10 * time.Second)

	client, err := seth.NewClientWithConfig(c.Cfg)
	require.NoError(t, err)

	gasBumps := 0

	client.GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
		gasBumps++
		return gasPrice
	}

	contractAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")

	// Send a transaction with a low gas price
	_, err = client.DeployContract(client.NewTXOpts(), "LinkToken", *contractAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.Error(t, err, "contract was deployed, but gas bumping is disabled")
	require.GreaterOrEqual(t, gasBumps, 0, "expected no gas bumps")
}

func TestGasBumping_Contract_Deployment_EIP_1559_SufficientBumping(t *testing.T) {
	c := newClient(t)

	// Set a low gas fee and tip cap and a short timeout
	c.Cfg.Network.GasTipCap = 1
	c.Cfg.Network.GasFeeCap = 1
	c.Cfg.Network.EIP1559DynamicFees = true
	c.Cfg.Network.TxnTimeout = seth.MustMakeDuration(10 * time.Second)
	c.Cfg.GasBumpRetries = 10

	client, err := seth.NewClientWithConfig(c.Cfg)
	require.NoError(t, err)

	gasBumps := 0

	client.GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
		gasBumps++
		newGasPrice := new(big.Int).Mul(gasPrice, big.NewInt(100))
		// cap max gas price to avoid hitting upper bound
		if newGasPrice.Cmp(big.NewInt(10000000)) >= 0 {
			return big.NewInt(0).Add(newGasPrice, big.NewInt(1000))
		}
		return newGasPrice
	}

	contractAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")

	// Send a transaction with a low gas price
	data, err := client.DeployContract(client.NewTXOpts(), "LinkToken", *contractAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.NoError(t, err, "contract wasn't deployed")
	require.GreaterOrEqual(t, gasBumps, 1, "expected at least one gas bump")
	require.Greater(t, data.Transaction.GasTipCap().Int64(), int64(1), "expected gas tip cap to be bumped")
	require.Greater(t, data.Transaction.GasFeeCap().Int64(), int64(1), "expected gas fee cap to be bumped")
}

func TestGasBumping_Contract_Deployment_EIP_1559_NonRootKey(t *testing.T) {
	c := newClient(t)

	// Set a low gas fee and tip cap and a short timeout
	c.Cfg.Network.GasTipCap = 1
	c.Cfg.Network.GasFeeCap = 1
	c.Cfg.Network.EIP1559DynamicFees = true
	c.Cfg.Network.TxnTimeout = seth.MustMakeDuration(10 * time.Second)
	c.Cfg.GasBumpRetries = 10
	var one int64 = 1
	c.Cfg.EphemeralAddrs = &one

	client, err := seth.NewClientWithConfig(c.Cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := client.NonceManager.UpdateNonces()
		require.NoError(t, err, "failed to update nonces")
		err = seth.ReturnFunds(client, client.Addresses[0].Hex())
		require.NoError(t, err, "failed to return funds")
	})

	gasBumps := 0

	client.GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
		gasBumps++
		newGasPrice := new(big.Int).Mul(gasPrice, big.NewInt(100))
		// cap max gas price to avoid hitting upper bound
		if newGasPrice.Cmp(big.NewInt(10000000)) >= 0 {
			return big.NewInt(0).Add(newGasPrice, big.NewInt(1000))
		}
		return newGasPrice
	}

	contractAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")

	// Send a transaction with a low gas price
	data, err := client.DeployContract(client.NewTXKeyOpts(1), "LinkToken", *contractAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.NoError(t, err, "contract wasn't deployed from key 1")
	require.GreaterOrEqual(t, gasBumps, 1, "expected at least one gas bump")
	require.Greater(t, data.Transaction.GasTipCap().Int64(), int64(1), "expected gas tip cap to be bumped")
	require.Greater(t, data.Transaction.GasFeeCap().Int64(), int64(1), "expected gas fee cap to be bumped")
}

func TestGasBumping_Contract_Deployment_EIP_1559_UnknownKey(t *testing.T) {
	c := newClient(t)

	// Set a low gas fee and tip cap and a short timeout
	c.Cfg.Network.GasTipCap = 1
	c.Cfg.Network.GasFeeCap = 1
	c.Cfg.Network.EIP1559DynamicFees = true
	c.Cfg.Network.TxnTimeout = seth.MustMakeDuration(10 * time.Second)
	c.Cfg.GasBumpRetries = 2
	var one int64 = 1
	c.Cfg.EphemeralAddrs = &one

	client, err := seth.NewClientWithConfig(c.Cfg)
	require.NoError(t, err)

	gasBumps := 0
	removedAddress := client.Addresses[1]

	client.GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
		// remove address from client to simulate an unlikely situation, where we try to bump a transaction with having sender's private key
		client.Addresses = client.Addresses[:1]
		gasBumps++
		return gasPrice
	}

	t.Cleanup(func() {
		client.Addresses = append(client.Addresses, removedAddress)
		err := client.NonceManager.UpdateNonces()
		require.NoError(t, err, "failed to update nonces")
		err = seth.ReturnFunds(client, client.Addresses[0].Hex())
		require.NoError(t, err, "failed to return funds")
	})

	contractAbi, err := link_token.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")

	_, err = client.DeployContract(client.NewTXKeyOpts(1), "LinkToken", *contractAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
	require.Error(t, err, "contract was deployed from unknown key")
	require.GreaterOrEqual(t, gasBumps, 1, "expected at least one gas bump attempt")
}

func TestGasBumping_Contract_Interaction_Legacy_SufficientBumping(t *testing.T) {
	spammer := test_utils.NewClientWithAddresses(t, 5)

	t.Cleanup(func() {
		err := spammer.NonceManager.UpdateNonces()
		require.NoError(t, err, "failed to update nonces")
		err = seth.ReturnFunds(spammer, spammer.Addresses[0].Hex())
		require.NoError(t, err, "failed to return funds")
	})

	var zero int64 = 0
	spammer.Cfg.EphemeralAddrs = &zero

	marshalled, err := toml.Marshal(spammer.Cfg)
	require.NoError(t, err)

	var configCopy seth.Config
	err = toml.Unmarshal(marshalled, &configCopy)
	require.NoError(t, err)

	configCopy.Network.DialTimeout = seth.MustMakeDuration(1 * time.Minute)

	client, err := seth.NewClientWithConfig(&configCopy)
	require.NoError(t, err)

	gasBumps := 0

	client.GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
		gasBumps++
		newGasPrice := new(big.Int).Mul(gasPrice, big.NewInt(100))
		// cap max gas price to avoid hitting upper bound
		if newGasPrice.Cmp(big.NewInt(100000000)) >= 0 {
			return big.NewInt(0).Add(newGasPrice, big.NewInt(1))
		}
		return newGasPrice
	}

	contractAbi, err := link_token_interface.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")

	// Send a transaction with a low gas price
	data, err := client.DeployContract(client.NewTXOpts(), "LinkToken", *contractAbi, common.FromHex(link_token_interface.LinkTokenMetaData.Bin))
	require.NoError(t, err, "contract wasn't deployed")

	linkContract, err := link_token.NewLinkToken(data.Address, client.Client)
	require.NoError(t, err, "failed to instantiate contract")

	// Update config and set a low gas price and a short timeout
	client.Cfg.Network.GasPrice = 1
	client.Cfg.Network.TxnTimeout = seth.MustMakeDuration(10 * time.Second)
	client.Cfg.GasBumpRetries = 10

	// introduce some traffic, so that bumping is necessary to mine the transaction
	go func() {
		for i := 0; i < 5; i++ {
			_, _ = spammer.DeployContract(spammer.NewTXKeyOpts(spammer.AnySyncedKey()), "LinkToken", *contractAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
		}
	}()

	_, err = client.Decode(linkContract.Transfer(client.NewTXOpts(), client.Addresses[0], big.NewInt(1000000000000000000)))
	require.NoError(t, err, "failed to mint tokens")
	require.GreaterOrEqual(t, gasBumps, 1, "expected at least one transaction gas bump")
}

func TestGasBumping_Contract_Interaction_Legacy_BumpingDisabled(t *testing.T) {
	spammer := test_utils.NewClientWithAddresses(t, 5)

	t.Cleanup(func() {
		err := spammer.NonceManager.UpdateNonces()
		require.NoError(t, err, "failed to update nonces")
		err = seth.ReturnFunds(spammer, spammer.Addresses[0].Hex())
		require.NoError(t, err, "failed to return funds")
	})

	var zero int64 = 0
	spammer.Cfg.EphemeralAddrs = &zero

	marshalled, err := toml.Marshal(spammer.Cfg)
	require.NoError(t, err)

	var configCopy seth.Config
	err = toml.Unmarshal(marshalled, &configCopy)
	require.NoError(t, err)

	configCopy.Network.DialTimeout = seth.MustMakeDuration(1 * time.Minute)

	client, err := seth.NewClientWithConfig(&configCopy)
	require.NoError(t, err)

	gasBumps := 0

	// do not bump anything
	client.GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
		gasBumps++
		return gasPrice
	}

	contractAbi, err := link_token_interface.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")

	// Send a transaction with a low gas price
	data, err := client.DeployContract(client.NewTXOpts(), "LinkToken", *contractAbi, common.FromHex(link_token_interface.LinkTokenMetaData.Bin))
	require.NoError(t, err, "contract wasn't deployed")

	linkContract, err := link_token.NewLinkToken(data.Address, client.Client)
	require.NoError(t, err, "failed to instantiate contract")

	// Update config and set a low gas price and a short timeout
	client.Cfg.Network.GasPrice = 1
	client.Cfg.Network.TxnTimeout = seth.MustMakeDuration(10 * time.Second)

	// introduce some traffic, so that bumping is necessary to mine the transaction
	go func() {
		for i := 0; i < 5; i++ {
			_, _ = spammer.DeployContract(spammer.NewTXKeyOpts(spammer.AnySyncedKey()), "LinkToken", *contractAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
		}
	}()

	_, err = client.Decode(linkContract.Transfer(client.NewTXOpts(), client.Addresses[0], big.NewInt(1000000000000000000)))
	require.Error(t, err, "did not fail to transfer tokens, even though gas bumping is disabled")
	require.Equal(t, gasBumps, 0, "expected no gas bumps")
}

func TestGasBumping_Contract_Interaction_Legacy_FailedBumping(t *testing.T) {
	spammer := test_utils.NewClientWithAddresses(t, 5)

	t.Cleanup(func() {
		err := spammer.NonceManager.UpdateNonces()
		require.NoError(t, err, "failed to update nonces")
		err = seth.ReturnFunds(spammer, spammer.Addresses[0].Hex())
		require.NoError(t, err, "failed to return funds")
	})

	var zero int64 = 0
	spammer.Cfg.EphemeralAddrs = &zero

	marshalled, err := toml.Marshal(spammer.Cfg)
	require.NoError(t, err)

	var configCopy seth.Config
	err = toml.Unmarshal(marshalled, &configCopy)
	require.NoError(t, err)

	configCopy.Network.DialTimeout = seth.MustMakeDuration(1 * time.Minute)

	client, err := seth.NewClientWithConfig(&configCopy)
	require.NoError(t, err)

	gasBumps := 0

	// this results in a gas bump that is too high to be accepted
	client.GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
		gasBumps++
		return new(big.Int).Mul(gasPrice, big.NewInt(1000000000000))
	}

	contractAbi, err := link_token_interface.LinkTokenMetaData.GetAbi()
	require.NoError(t, err, "failed to get ABI")

	// Send a transaction with a low gas price
	data, err := client.DeployContract(client.NewTXOpts(), "LinkToken", *contractAbi, common.FromHex(link_token_interface.LinkTokenMetaData.Bin))
	require.NoError(t, err, "contract wasn't deployed")

	linkContract, err := link_token.NewLinkToken(data.Address, client.Client)
	require.NoError(t, err, "failed to instantiate contract")

	// Update config and set a low gas price and a short timeout
	client.Cfg.Network.GasPrice = 1
	client.Cfg.Network.TxnTimeout = seth.MustMakeDuration(10 * time.Second)
	client.Cfg.GasBumpRetries = 3

	// introduce some traffic, so that bumping is necessary to mine the transaction
	go func() {
		for i := 0; i < 5; i++ {
			_, _ = spammer.DeployContract(spammer.NewTXKeyOpts(spammer.AnySyncedKey()), "LinkToken", *contractAbi, common.FromHex(link_token.LinkTokenMetaData.Bin))
		}
	}()

	_, err = client.Decode(linkContract.Transfer(client.NewTXOpts(), client.Addresses[0], big.NewInt(1000000000000000000)))
	require.Error(t, err, "did not fail to transfer tokens, even though gas bumping is disabled")
	require.Equal(t, 3, gasBumps, "expected 2 gas bumps")
}
