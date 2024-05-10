package seth

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
)

var INSUFFICIENT_EPHEMERAL_KEYS = `
Error: Insufficient Ephemeral Addresses for Simulated Network

To operate on a simulated network, you must configure at least one ephemeral address. Currently, %d ephemeral address(es) are set. Please update your TOML configuration file as follows to meet this requirement:
[Seth] ephemeral_addresses_number = 1

This adjustment ensures that your setup is minimally viable. Although it is highly recommended to use at least 20 ephemeral addresses.
`

var INSUFFICIENT_STATIC_KEYS = `
Error: Insufficient Private Keys for Live Network

To run this test on a live network, you must either:
1. Set at least two private keys in the '[Network.WalletKeys]' section of your TOML configuration file. Example format:
   [Network.WalletKeys]
   NETWORK_NAME=["PRIVATE_KEY_1", "PRIVATE_KEY_2"]
2. Set at least two private keys in the '[Network.EVMNetworks.NETWORK_NAME] section of your TOML configuration file. Example format:
   evm_keys=["PRIVATE_KEY_1", "PRIVATE_KEY_2"]

Currently, only %d private key/s is/are set.

Recommended Action:
Distribute your funds across multiple private keys and update your configuration accordingly. Even though 1 private key is sufficient for testing, it is highly recommended to use at least 10 private keys.
`

// GetAndAssertCorrectConcurrency checks Seth configuration for the number of ephemeral keys or static keys (depending on Seth configuration) and makes sure that
// the number is at least minConcurrency. If the number is less than minConcurrency, it returns an error. The root key is always excluded from the count.
func (m *Client) GetAndAssertCorrectConcurrency(minConcurrency int) (int, error) {
	concurrency := m.Cfg.GetMaxConcurrency()

	var msg string
	if m.Cfg.IsSimulatedNetwork() {
		msg = fmt.Sprintf(INSUFFICIENT_EPHEMERAL_KEYS, concurrency)
	} else {
		msg = fmt.Sprintf(INSUFFICIENT_STATIC_KEYS, concurrency)
	}

	if concurrency < minConcurrency {
		return 0, fmt.Errorf(msg)
	}

	return concurrency, nil
}

// MustGetRootKeyAddress returns the root key address from the client configuration. If no addresses are found, it panics.
// Root key address is the first address in the list of addresses.
func (m *Client) MustGetRootKeyAddress() common.Address {
	if len(m.Addresses) == 0 {
		panic("No addresses found in the client configuration")
	}
	return m.Addresses[0]
}

// GetRootKeyAddress returns the root key address from the client configuration. If no addresses are found, it returns an error.
// Root key address is the first address in the list of addresses.
func (m *Client) GetRootKeyAddress() (common.Address, error) {
	if len(m.Addresses) == 0 {
		return common.Address{}, errors.New("No addresses found in the client configuration")
	}
	return m.Addresses[0], nil
}
