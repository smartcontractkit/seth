package seth

import (
	"errors"
	"github.com/ethereum/go-ethereum/common"
)

// MustGetRootKeyAddress returns the root key address from the client configuration. If no addresses are found, it panics.
// Root key address is the first address in the list of addresses.
func (m *Client) MustGetRootKeyAddress() common.Address {
	if len(m.Addresses) == 0 {
		panic("no addresses found in the client configuration")
	}
	return m.Addresses[0]
}

// GetRootKeyAddress returns the root key address from the client configuration. If no addresses are found, it returns an error.
// Root key address is the first address in the list of addresses.
func (m *Client) GetRootKeyAddress() (common.Address, error) {
	if len(m.Addresses) == 0 {
		return common.Address{}, errors.New("no addresses found in the client configuration")
	}
	return m.Addresses[0], nil
}
