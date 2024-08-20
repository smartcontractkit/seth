package seth

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
)

/* these are the common errors of RPCs */

const (
	ErrRPCConnectionRefused = "connection refused"
)

const (
	ErrRetryTimeout = "retry timeout"
)

// RetryTxAndDecode executes transaction several times, retries if connection is lost and decodes all the data
func (m *Client) RetryTxAndDecode(f func() (*types.Transaction, error)) (*DecodedTransaction, error) {
	var tx *types.Transaction
	err := retry.Do(
		func() error {
			var err error
			tx, err = f()
			return err
		}, retry.OnRetry(func(i uint, _ error) {
			L.Debug().Uint("Attempt", i).Msg("Retrying transaction...")
		}),
		retry.DelayType(retry.FixedDelay),
		retry.Attempts(10), retry.Delay(time.Duration(1)*time.Second), retry.RetryIf(func(err error) bool {
			return strings.Contains(err.Error(), ErrRPCConnectionRefused)
		}),
	)

	if err != nil {
		return &DecodedTransaction{}, errors.New(ErrRetryTimeout)
	}

	dt, err := m.Decode(tx, nil)
	if err != nil {
		return &DecodedTransaction{}, errors.Wrap(err, "error decoding transaction")
	}

	return dt, nil
}

type GasBumpStrategyFn = func(gasPrice *big.Int) *big.Int

var NoOpGasBumpStrategyFn = func(gasPrice *big.Int) *big.Int {
	return gasPrice
}

var bumpGasOnTimeout = func(client *Client, tx *types.Transaction) (*types.Transaction, error) {
	L.Warn().Msgf("Transaction wasn't confirmed in %s. Bumping gas", client.Cfg.Network.TxnTimeout.String())

	ctx, cancel := context.WithTimeout(context.Background(), client.Cfg.Network.TxnTimeout.Duration())
	_, isPending, err := client.Client.TransactionByHash(ctx, tx.Hash())
	cancel()
	if err != nil {
		return nil, err
	}

	if !isPending {
		L.Debug().Str("Tx hash", tx.Hash().Hex()).Msg("Transaction was confirmed before bumping gas")
		return tx, nil
	}

	signer := types.LatestSignerForChainID(tx.ChainId())
	sender, err := types.Sender(signer, tx)
	if err != nil {
		return nil, err
	}

	senderPkIdx := -1
	for j, maybeSender := range client.Addresses {
		if maybeSender == sender {
			senderPkIdx = j
			break
		}
	}

	if senderPkIdx == -1 {
		return nil, fmt.Errorf("sender address '%s' not found in loaded private keys", sender)
	}

	privateKey := client.PrivateKeys[senderPkIdx]

	var replacementTx *types.Transaction

	// Legacy tx
	switch tx.Type() {
	case types.LegacyTxType:
		gasPrice := client.GasBumpStrategyFn(tx.GasPrice())
		L.Warn().Interface("Old gas price", tx.GasPrice()).Interface("New gas price", gasPrice).Msg("Bumping gas price for legacy transaction")
		txData := &types.LegacyTx{
			Nonce:    tx.Nonce(),
			To:       tx.To(),
			Value:    tx.Value(),
			Gas:      tx.Gas(),
			GasPrice: gasPrice,
			Data:     tx.Data(),
		}
		replacementTx, err = types.SignNewTx(privateKey, signer, txData)
	case types.DynamicFeeTxType:
		gasFeeCap := client.GasBumpStrategyFn(tx.GasFeeCap())
		gasTipCap := client.GasBumpStrategyFn(tx.GasTipCap())
		L.Warn().Interface("Old gas fee cap", tx.GasFeeCap()).Interface("New gas fee cap", gasFeeCap).Interface("Old gas tip cap", tx.GasTipCap()).Interface("New gas tip cap", gasTipCap).Msg("Bumping gas fee cap and tip cap for EIP-1559 transaction")
		txData := &types.DynamicFeeTx{
			Nonce:     tx.Nonce(),
			To:        tx.To(),
			Value:     tx.Value(),
			Gas:       tx.Gas(),
			GasFeeCap: gasFeeCap,
			GasTipCap: gasTipCap,
			Data:      tx.Data(),
		}

		replacementTx, err = types.SignNewTx(privateKey, signer, txData)
	default:
		return nil, fmt.Errorf("unsupported tx type %d", tx.Type())
	}

	if err != nil {
		return nil, err
	}

	ctx, cancel = context.WithTimeout(context.Background(), client.Cfg.Network.TxnTimeout.Duration())
	defer cancel()
	err = client.Client.SendTransaction(ctx, replacementTx)
	// contrary to convention we return initial tx here, so that next retry will bump gas again using original tx
	// what could have happened here is that the tx was mined in the meantime and if that happened we need to have the original tx hash
	// we do not want to check for explicit error here, like 'nonce too low', because it might differ for each Ethereum client
	if err != nil {
		return tx, err
	}

	return replacementTx, nil
}
