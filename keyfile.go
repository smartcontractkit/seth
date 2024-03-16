package seth

import (
	"context"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/naoina/toml"
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
	suggestedGasTipCap, err := c.Client.SuggestGasTipCap(context.Background())
	if err != nil {
		return err
	}
	bd, err := c.CalculateSubKeyFunding(opts.Addrs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eg, egCtx := errgroup.WithContext(ctx)
	for _, kfd := range keyFile.Keys {
		kfd := kfd
		eg.Go(func() error {
			kfd.Funds = bd.AddrFunding.String()
			return c.TransferETHFromKey(egCtx, 0, kfd.Address, bd.AddrFunding, suggestedGasTipCap)
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

// ReturnFunds returns funds to the root key from all the test keys in some "keyfile.toml"
func ReturnFunds(c *Client, toAddr string) error {
	if toAddr == "" {
		toAddr = c.Addresses[0].Hex()
	}
	suggestedGasTipCap, err := c.Client.SuggestGasTipCap(context.Background())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eg, egCtx := errgroup.WithContext(ctx)
	for i := 1; i < len(c.Addresses); i++ {
		i := i
		eg.Go(func() error {
			balance, err := c.Client.BalanceAt(context.Background(), c.Addresses[i], nil)
			if err != nil {
				return err
			}
			networkTransferFee := c.Cfg.Network.GasPrice * c.Cfg.Network.TransferGasFee
			fundsToReturn := new(big.Int).Sub(balance, big.NewInt(networkTransferFee))
			L.Info().
				Str("Key", c.Addresses[i].Hex()).
				Interface("Balance", balance).
				Interface("NetworkFee", c.Cfg.Network.GasPrice*21).
				Interface("ReturnedFunds", fundsToReturn).
				Msg("KeyFile key balance")
			return c.TransferETHFromKey(egCtx, i, toAddr, fundsToReturn, suggestedGasTipCap)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	keyFile, err := c.CreateOrUnmarshalKeyFile(nil)
	if err != nil {
		return err
	}
	for _, kfd := range keyFile.Keys {
		kfd := kfd
		eg.Go(func() error {
			balance, err := c.Client.BalanceAt(context.Background(), common.HexToAddress(kfd.Address), nil)
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
