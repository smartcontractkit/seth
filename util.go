package seth

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/naoina/toml"
	"github.com/pkg/errors"
	network_debug_contract "github.com/smartcontractkit/seth/contracts/bind/debug"
	network_sub_debug_contract "github.com/smartcontractkit/seth/contracts/bind/sub"
)

const (
	ErrEmptyKeyFile               = "keyfile is empty"
	ErrInsufficientRootKeyBalance = "insufficient root key balance: %s"
)

// KeyFile is a struct that holds all test keys data
type KeyFile struct {
	Keys []*KeyData `toml:"keys"`
}

// KeyData data for test keys
type KeyData struct {
	PrivateKey string `toml:"private_key"`
	Address    string `toml:"address"`
	Funds      string `toml:"funds"`
}

// FundKeyFileCmdOpts funding params for CLI
type FundKeyFileCmdOpts struct {
	Addrs int64
}

// FundingDetails funding details about shares we put into test keys
type FundingDetails struct {
	RootBalance        *big.Int
	TotalFee           *big.Int
	FreeBalance        *big.Int
	AddrFunding        *big.Int
	NetworkTransferFee int64
}

// NewEphemeralKeys creates a new ephemeral keyfile, can be used for simulated networks
func NewEphemeralKeys(addrs int64) ([]string, error) {
	privKeys := make([]string, 0)
	for i := 0; i < int(addrs); i++ {
		_, pKey, err := NewAddress()
		if err != nil {
			return nil, err
		}
		privKeys = append(privKeys, pKey)
	}
	return privKeys, nil
}

// CalculateSubKeyFunding calculates all required params to split funds from the root key to N test keys
func (m *Client) CalculateSubKeyFunding(addrs int64) (*FundingDetails, error) {
	balance, err := m.Client.BalanceAt(context.Background(), m.Addresses[0], nil)
	if err != nil {
		return nil, err
	}
	L.Info().Str("Balance", balance.String()).Msg("Root key balance")
	networkTransferFee := m.Cfg.Network.GasPrice * m.Cfg.Network.TransferGasFee
	totalFee := new(big.Int).Mul(big.NewInt(networkTransferFee), big.NewInt(addrs))
	freeBalance := new(big.Int).Sub(balance, totalFee)
	addrFunding := new(big.Int).Div(freeBalance, big.NewInt(addrs))
	bd := &FundingDetails{
		RootBalance:        balance,
		TotalFee:           totalFee,
		FreeBalance:        freeBalance,
		AddrFunding:        addrFunding,
		NetworkTransferFee: networkTransferFee,
	}
	L.Info().
		Interface("RootBalance", bd.RootBalance.String()).
		Interface("TransferFeesTotal", bd.TotalFee.String()).
		Interface("NetworkTransferFee", bd.NetworkTransferFee).
		Interface("FreeBalance", bd.FreeBalance.String()).
		Interface("EachAddrGets", bd.AddrFunding.String()).
		Msg("Splitting funds from the root account")
	if freeBalance.Cmp(big.NewInt(0)) <= 0 {
		return nil, errors.New(fmt.Sprintf(ErrInsufficientRootKeyBalance, freeBalance.String()))
	}
	return bd, nil
}

func (m *Client) CreateOrUnmarshalKeyFile(opts *FundKeyFileCmdOpts) (*KeyFile, error) {
	if _, err := os.Stat(m.Cfg.KeyFilePath); os.IsNotExist(err) {
		L.Info().
			Str("Path", m.Cfg.KeyFilePath).
			Interface("Opts", opts).
			Msg("Creating a new key file")
		if _, err := os.Create(m.Cfg.KeyFilePath); err != nil {
			return nil, err
		}
		kf := NewKeyFile()
		for i := 0; i < int(opts.Addrs); i++ {
			addr, pKey, err := NewAddress()
			if err != nil {
				return nil, err
			}
			kf.Keys = append(kf.Keys, &KeyData{PrivateKey: pKey, Address: addr})
		}
		return kf, nil
	} else {
		L.Info().
			Str("Path", m.Cfg.KeyFilePath).
			Interface("Opts", opts).
			Msg("Loading keyfile")
		var kf *KeyFile
		d, err := os.ReadFile(m.Cfg.KeyFilePath)
		if err != nil {
			return nil, err
		}
		if err := toml.Unmarshal(d, &kf); err != nil {
			return nil, err
		}
		if len(kf.Keys) == 0 {
			return nil, errors.New(ErrEmptyKeyFile)
		}
		return kf, nil
	}
}

func (m *Client) DeployDebugSubContract() (*network_sub_debug_contract.NetworkDebugSubContract, common.Address, error) {
	address, tx, instance, err := network_sub_debug_contract.DeployNetworkDebugSubContract(m.NewTXOpts(), m.Client)
	if err != nil {
		return nil, common.Address{}, err
	}
	L.Info().
		Str("Address", address.Hex()).
		Str("TXHash", tx.Hash().Hex()).
		Msg("Deploying sub-debug contract")
	if _, err := bind.WaitDeployed(context.Background(), m.Client, tx); err != nil {
		return nil, common.Address{}, err
	}
	L.Info().
		Str("Address", address.Hex()).
		Str("TXHash", tx.Hash().Hex()).
		Msg("Sub-debug contract deployed")
	return instance, address, nil
}

func (m *Client) DeployDebugContract(subDbgAddr common.Address) (*network_debug_contract.NetworkDebugContract, common.Address, error) {
	address, tx, instance, err := network_debug_contract.DeployNetworkDebugContract(m.NewTXOpts(), m.Client, subDbgAddr)
	if err != nil {
		return nil, common.Address{}, err
	}
	L.Info().
		Str("Address", address.Hex()).
		Str("TXHash", tx.Hash().Hex()).
		Msg("Deploying debug contract")
	if _, err := bind.WaitDeployed(context.Background(), m.Client, tx); err != nil {
		return nil, common.Address{}, err
	}
	L.Info().
		Str("Address", address.Hex()).
		Str("TXHash", tx.Hash().Hex()).
		Msg("Debug contract deployed")
	return instance, address, nil
}

func NewKeyFile() *KeyFile {
	return &KeyFile{Keys: make([]*KeyData, 0)}
}

// Duration is a non-negative time duration.
type Duration struct{ d time.Duration }

func MakeDuration(d time.Duration) (Duration, error) {
	if d < time.Duration(0) {
		return Duration{}, fmt.Errorf("cannot make negative time duration: %s", d)
	}
	return Duration{d: d}, nil
}

func ParseDuration(s string) (Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return Duration{}, err
	}

	return MakeDuration(d)
}

func MustMakeDuration(d time.Duration) *Duration {
	rv, err := MakeDuration(d)
	if err != nil {
		panic(err)
	}
	return &rv
}

// Duration returns the value as the standard time.Duration value.
func (d Duration) Duration() time.Duration {
	return d.d
}

// Before returns the time d units before time t
func (d Duration) Before(t time.Time) time.Time {
	return t.Add(-d.Duration())
}

// Shorter returns true if and only if d is shorter than od.
func (d Duration) Shorter(od Duration) bool { return d.d < od.d }

// IsInstant is true if and only if d is of duration 0
func (d Duration) IsInstant() bool { return d.d == 0 }

// String returns a string representing the duration in the form "72h3m0.5s".
// Leading zero units are omitted. As a special case, durations less than one
// second format use a smaller unit (milli-, micro-, or nanoseconds) to ensure
// that the leading digit is non-zero. The zero duration formats as 0s.
func (d Duration) String() string {
	return d.Duration().String()
}

// MarshalJSON implements the json.Marshaler interface.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (d *Duration) UnmarshalJSON(input []byte) error {
	var txt string
	err := json.Unmarshal(input, &txt)
	if err != nil {
		return err
	}
	v, err := time.ParseDuration(string(txt))
	if err != nil {
		return err
	}
	*d, err = MakeDuration(v)
	if err != nil {
		return err
	}
	return nil
}

func (d *Duration) Scan(v interface{}) (err error) {
	switch tv := v.(type) {
	case int64:
		*d, err = MakeDuration(time.Duration(tv))
		return err
	default:
		return errors.Errorf(`don't know how to parse "%s" of type %T as a `+
			`models.Duration`, tv, tv)
	}
}

func (d Duration) Value() (driver.Value, error) {
	return int64(d.d), nil
}

// MarshalText implements the text.Marshaler interface.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.d.String()), nil
}

// UnmarshalText implements the text.Unmarshaler interface.
func (d *Duration) UnmarshalText(input []byte) error {
	v, err := time.ParseDuration(string(input))
	if err != nil {
		return err
	}
	pd, err := MakeDuration(v)
	if err != nil {
		return err
	}
	*d = pd
	return nil
}

func saveAsJson(v any, dirName, name string) (string, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := fmt.Sprintf("%s/%s", pwd, dirName)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(dir, os.ModePerm)
		if err != nil {
			return "", err
		}
	}
	confPath := fmt.Sprintf("%s/%s.json", dir, name)
	f, _ := json.MarshalIndent(v, "", "   ")
	err = os.WriteFile(confPath, f, 0600)

	return confPath, err
}

func OpenJsonFileAsStruct(path string, v any) error {
	jsonFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer jsonFile.Close()
	b, _ := io.ReadAll(jsonFile)
	err = json.Unmarshal(b, v)
	if err != nil {
		return err
	}
	return nil
}

// CreateOrAppendToJsonArray appends to a JSON array in a file or creates a new JSON array if the file is empty or doesn't exist
func CreateOrAppendToJsonArray(filePath string, newItem any) error {
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	jsonBytes, err := json.Marshal(newItem)
	if err != nil {
		return err
	}
	jsonValue := string(jsonBytes)

	if size == 0 {
		_, err = f.WriteString(fmt.Sprintf("[%s]", jsonValue))
	} else {
		// Move cursor back by one character, so we can append data just before array end.
		_, err = f.Seek(-1, io.SeekEnd)
		if err != nil {
			return err
		}
		_, err = f.WriteString(fmt.Sprintf(",\n%s]", jsonValue))
	}
	return err
}
