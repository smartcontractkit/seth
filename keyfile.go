package seth

import (
	"context"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/errgroup"
)

// NewAddress creates a new address
func NewAddress() (string, string, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return "", "", err
	}
	privateKeyBytes := crypto.FromECDSA(privateKey)
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return "", "", errors.New("error casting public key to ECDSA")
	}
	address := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	L.Info().
		Str("Addr", address).
		Msg("New address created")
	return address, hexutil.Encode(privateKeyBytes)[2:], nil
}

// UpdateAndSplitFunds splits funds from the root key into equal parts
func UpdateAndSplitFunds(c *Client, opts *FundKeyFileCmdOpts) error {
	keyFile, err := c.CreateOrUnmarshalKeyFile(opts)
	if err != nil {
		return err
	}

	gasPrice, err := c.GetSuggestedLegacyFees(context.Background(), Priority_Standard)
	if err != nil {
		gasPrice = big.NewInt(c.Cfg.Network.GasPrice)
	}

	bd, err := c.CalculateSubKeyFunding(opts.Addrs, gasPrice.Int64(), opts.RootKeyBuffer)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eg, egCtx := errgroup.WithContext(ctx)
	for _, kfd := range keyFile.Keys {
		kfd := kfd
		eg.Go(func() error {
			err := c.TransferETHFromKey(egCtx, 0, kfd.Address, bd.AddrFunding, gasPrice)
			if err != nil {
				return err
			}
			bal, err := c.Client.BalanceAt(egCtx, common.HexToAddress(kfd.Address), nil)
			if err != nil {
				return err
			}
			kfd.Funds = bal.String()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	b, err := toml.Marshal(keyFile)
	if err != nil {
		return err
	}
	return os.WriteFile(c.Cfg.KeyFilePath, b, os.ModePerm)
}

// ReturnFundsAndUpdateKeyfile returns funds to the root key from all other keys
func ReturnFunds(c *Client, toAddr string) error {
	if toAddr == "" {
		toAddr = c.Addresses[0].Hex()
	}

	gasPrice, err := c.GetSuggestedLegacyFees(context.Background(), Priority_Standard)
	if err != nil {
		gasPrice = big.NewInt(c.Cfg.Network.GasPrice)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eg, egCtx := errgroup.WithContext(ctx)

	if len(c.Addresses) == 1 {
		return errors.New("No addresses to return funds from. Have you passed correct key file?")
	}

	for i := 1; i < len(c.Addresses); i++ {
		idx := i
		eg.Go(func() error {
			balance, err := c.Client.BalanceAt(context.Background(), c.Addresses[idx], nil)
			if err != nil {
				L.Error().Err(err).Msg("Error getting balance")
				return err
			}

			var gasLimit int64
			gasLimitRaw, err := c.EstimateGasLimitForFundTransfer(c.Addresses[idx], common.HexToAddress(toAddr), balance)
			if err != nil {
				gasLimit = c.Cfg.Network.TransferGasFee
			} else {
				gasLimit = int64(gasLimitRaw)
			}

			networkTransferFee := gasPrice.Int64() * gasLimit
			fundsToReturn := new(big.Int).Sub(balance, big.NewInt(networkTransferFee))

			if fundsToReturn.Cmp(big.NewInt(0)) == -1 {
				L.Warn().
					Str("Key", c.Addresses[idx].Hex()).
					Interface("Balance", balance).
					Interface("NetworkFee", networkTransferFee).
					Interface("FundsToReturn", fundsToReturn).
					Msg("Insufficient funds to return. Skipping.")
				return nil
			}

			L.Info().
				Str("Key", c.Addresses[idx].Hex()).
				Interface("Balance", balance).
				Interface("NetworkFee", c.Cfg.Network.GasPrice*gasLimit).
				Interface("GasLimit", gasLimit).
				Interface("GasPrice", gasPrice).
				Interface("FundsToReturn", fundsToReturn).
				Msg("KeyFile key balance")

			return c.TransferETHFromKey(
				egCtx,
				idx,
				toAddr,
				fundsToReturn,
				gasPrice,
			)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

// ReturnFundsAndUpdateKeyfile returns funds to the root key from all the test keys in some "keyfile.toml"
func ReturnFundsAndUpdateKeyfile(c *Client, toAddr string) error {
	err := ReturnFunds(c, toAddr)
	if err != nil {
		return err
	}
	keyFile, err := c.CreateOrUnmarshalKeyFile(nil)
	if err != nil {
		return err
	}
	eg, egCtx := errgroup.WithContext(context.Background())
	for _, kfd := range keyFile.Keys {
		kfd := kfd
		eg.Go(func() error {
			balance, err := c.Client.BalanceAt(egCtx, common.HexToAddress(kfd.Address), nil)
			if err != nil {
				return err
			}
			kfd.Funds = balance.String()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	b, err := toml.Marshal(keyFile)
	if err != nil {
		return err
	}
	return os.WriteFile(c.Cfg.KeyFilePath, b, os.ModePerm)
}

// UpdateKeyFileBalances updates file balances
func UpdateKeyFileBalances(c *Client) error {
	keyFile, err := c.CreateOrUnmarshalKeyFile(nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eg, egCtx := errgroup.WithContext(ctx)
	for _, kfd := range keyFile.Keys {
		kfd := kfd
		eg.Go(func() error {
			balance, err := c.Client.BalanceAt(egCtx, common.HexToAddress(kfd.Address), nil)
			if err != nil {
				return err
			}
			kfd.Funds = balance.String()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	b, err := toml.Marshal(keyFile)
	if err != nil {
		return err
	}
	return os.WriteFile(c.Cfg.KeyFilePath, b, os.ModePerm)
}
