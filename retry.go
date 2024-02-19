package seth

import (
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
