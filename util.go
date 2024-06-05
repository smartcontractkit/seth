package seth

import (
	"context"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/pelletier/go-toml/v2"
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
	Addrs         int64
	RootKeyBuffer int64
	LocalKeyfile  bool
	VaultId       string
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
func (m *Client) CalculateSubKeyFunding(addrs, gasPrice, rooKeyBuffer int64) (*FundingDetails, error) {
	balance, err := m.Client.BalanceAt(context.Background(), m.Addresses[0], nil)
	if err != nil {
		return nil, err
	}

	gasLimit := m.Cfg.Network.TransferGasFee
	newAddress, _, err := NewAddress()
	if err == nil {
		gasLimitRaw, err := m.EstimateGasLimitForFundTransfer(m.Addresses[0], common.HexToAddress(newAddress), big.NewInt(0).Quo(balance, big.NewInt(addrs)))
		if err == nil {
			gasLimit = int64(gasLimitRaw)
		}
	}

	networkTransferFee := gasPrice * gasLimit
	totalFee := new(big.Int).Mul(big.NewInt(networkTransferFee), big.NewInt(addrs))
	rootKeyBuffer := new(big.Int).Mul(big.NewInt(rooKeyBuffer), big.NewInt(1_000_000_000_000_000_000))
	freeBalance := new(big.Int).Sub(balance, big.NewInt(0).Add(totalFee, rootKeyBuffer))

	L.Info().
		Str("Balance (wei/ether)", fmt.Sprintf("%s/%s", balance.String(), WeiToEther(balance).Text('f', -1))).
		Str("Total fee (wei/ether)", fmt.Sprintf("%s/%s", totalFee.String(), WeiToEther(totalFee).Text('f', -1))).
		Str("Free Balance (wei/ether)", fmt.Sprintf("%s/%s", freeBalance.String(), WeiToEther(freeBalance).Text('f', -1))).
		Str("Buffer (wei/ether)", fmt.Sprintf("%s/%s", rootKeyBuffer.String(), WeiToEther(rootKeyBuffer).Text('f', -1))).
		Msg("Root key balance")

	if freeBalance.Cmp(big.NewInt(0)) < 0 {
		return nil, errors.New(fmt.Sprintf(ErrInsufficientRootKeyBalance, freeBalance.String()))
	}

	addrFunding := new(big.Int).Div(freeBalance, big.NewInt(addrs))
	requiredBalance := big.NewInt(0).Mul(addrFunding, big.NewInt(addrs))

	L.Debug().
		Str("Funding per ephemeral key (wei/ether)", fmt.Sprintf("%s/%s", addrFunding.String(), WeiToEther(addrFunding).Text('f', -1))).
		Str("Available balance (wei/ether)", fmt.Sprintf("%s/%s", freeBalance.String(), WeiToEther(freeBalance).Text('f', -1))).
		Interface("Required balance (wei/ether)", fmt.Sprintf("%s/%s", requiredBalance.String(), WeiToEther(requiredBalance).Text('f', -1))).
		Msg("Using hardcoded ephemeral funding")

	if freeBalance.Cmp(requiredBalance) < 0 {
		return nil, errors.New(fmt.Sprintf(ErrInsufficientRootKeyBalance, freeBalance.String()))
	}

	bd := &FundingDetails{
		RootBalance:        balance,
		TotalFee:           totalFee,
		FreeBalance:        freeBalance,
		AddrFunding:        addrFunding,
		NetworkTransferFee: networkTransferFee,
	}
	L.Info().
		Interface("RootBalance", bd.RootBalance.String()).
		Interface("RootKeyBuffer", rootKeyBuffer.String()).
		Interface("TransferFeesTotal", bd.TotalFee.String()).
		Interface("NetworkTransferFee", bd.NetworkTransferFee).
		Interface("FreeBalance", bd.FreeBalance.String()).
		Interface("EachAddrGets", bd.AddrFunding.String()).
		Msg("Splitting funds from the root account")

	return bd, nil
}

type KeyfileStatus = bool

const (
	NewKeyfile      KeyfileStatus = true
	ExistingKeyfile KeyfileStatus = false
)

func (m *Client) CreateOrUnmarshalKeyFile(opts *FundKeyFileCmdOpts) (*KeyFile, KeyfileStatus, error) {
	if opts.LocalKeyfile {
		if _, err := os.Stat(m.Cfg.KeyFilePath); os.IsNotExist(err) {
			L.Info().
				Str("Path", m.Cfg.KeyFilePath).
				Interface("Opts", opts).
				Msg("Creating a new key file")
			if _, err := os.Create(m.Cfg.KeyFilePath); err != nil {
				return nil, NewKeyfile, err
			}

			kf := NewKeyFile()
			for i := 0; i < int(opts.Addrs); i++ {
				addr, pKey, err := NewAddress()
				if err != nil {
					return nil, false, err
				}
				kf.Keys = append(kf.Keys, &KeyData{PrivateKey: pKey, Address: addr})
			}
			return kf, NewKeyfile, nil
		} else {
			L.Info().
				Str("Path", m.Cfg.KeyFilePath).
				Interface("Opts", opts).
				Msg("Loading keyfile. Ignoring addresses-related opts")
			var kf *KeyFile

			d, err := os.ReadFile(m.Cfg.KeyFilePath)
			if err != nil {
				return nil, false, err
			}
			if err := toml.Unmarshal(d, &kf); err != nil {
				return nil, false, err
			}

			if kf == nil || len(kf.Keys) == 0 {
				return nil, false, errors.New(ErrEmptyKeyFile)
			}
			return kf, ExistingKeyfile, nil
		}
	} else {
		existsIn1Pass, err := ExistsIn1Pass(m, opts.VaultId)
		if err != nil {
			L.Error().Err(err).Msg("error trying to check if keyfile exists in 1Password")
			return nil, false, err
		}

		if existsIn1Pass {
			keyfile, err := LoadFrom1Pass(m, opts.VaultId)
			if err != nil {
				return &KeyFile{}, false, err
			}
			return &keyfile, ExistingKeyfile, nil
		}

		kf := NewKeyFile()
		for i := 0; i < int(opts.Addrs); i++ {
			addr, pKey, err := NewAddress()
			if err != nil {
				return nil, false, err
			}
			kf.Keys = append(kf.Keys, &KeyData{PrivateKey: pKey, Address: addr})
		}
		return kf, NewKeyfile, nil
	}
	//
	//if _, err := os.Stat(m.Cfg.KeyFilePath); os.IsNotExist(err) {
	//	L.Info().
	//		Str("Path", m.Cfg.KeyFilePath).
	//		Interface("Opts", opts).
	//		Msg("Creating a new key file")
	//	if opts.LocalKeyfile {
	//		if _, err := os.Create(m.Cfg.KeyFilePath); err != nil {
	//			return nil, NewKeyfile, err
	//		}
	//	}
	//	kf := NewKeyFile()
	//	for i := 0; i < int(opts.Addrs); i++ {
	//		addr, pKey, err := NewAddress()
	//		if err != nil {
	//			return nil, false, err
	//		}
	//		kf.Keys = append(kf.Keys, &KeyData{PrivateKey: pKey, Address: addr})
	//	}
	//	return kf, NewKeyfile, nil
	//} else {
	//	L.Info().
	//		Str("Path", m.Cfg.KeyFilePath).
	//		Interface("Opts", opts).
	//		Msg("Loading keyfile. Ignoring addresses-related opts")
	//	var kf *KeyFile
	//
	//	if opts.LocalKeyfile {
	//		d, err := os.ReadFile(m.Cfg.KeyFilePath)
	//		if err != nil {
	//			return nil, false, err
	//		}
	//		if err := toml.Unmarshal(d, &kf); err != nil {
	//			return nil, false, err
	//		}
	//	} else {
	//		keyfile, err := LoadFrom1Pass(m, opts.VaultId)
	//		if err != nil {
	//			return kf, false, err
	//		}
	//		kf = &keyfile
	//	}
	//
	//	if kf == nil || len(kf.Keys) == 0 {
	//		return nil, false, errors.New(ErrEmptyKeyFile)
	//	}
	//	return kf, ExistingKeyfile, nil
	//}
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
type Duration struct{ D time.Duration }

func MakeDuration(d time.Duration) (Duration, error) {
	if d < time.Duration(0) {
		return Duration{}, fmt.Errorf("cannot make negative time duration: %s", d)
	}
	return Duration{D: d}, nil
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
	return d.D
}

// Before returns the time d units before time t
func (d Duration) Before(t time.Time) time.Time {
	return t.Add(-d.Duration())
}

// Shorter returns true if and only if d is shorter than od.
func (d Duration) Shorter(od Duration) bool { return d.D < od.D }

// IsInstant is true if and only if d is of duration 0
func (d Duration) IsInstant() bool { return d.D == 0 }

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
	return int64(d.D), nil
}

// MarshalText implements the text.Marshaler interface.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.D.String()), nil
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

// EtherToWei converts an ETH float amount to wei
func EtherToWei(eth *big.Float) *big.Int {
	truncInt, _ := eth.Int(nil)
	truncInt = new(big.Int).Mul(truncInt, big.NewInt(params.Ether))
	fracStr := strings.Split(fmt.Sprintf("%.18f", eth), ".")[1]
	fracStr += strings.Repeat("0", 18-len(fracStr))
	fracInt, _ := new(big.Int).SetString(fracStr, 10)
	wei := new(big.Int).Add(truncInt, fracInt)
	return wei
}

// WeiToEther converts a wei amount to eth float
func WeiToEther(wei *big.Int) *big.Float {
	f := new(big.Float)
	f.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	f.SetMode(big.ToNearestEven)
	fWei := new(big.Float)
	fWei.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	fWei.SetMode(big.ToNearestEven)
	return f.Quo(fWei.SetInt(wei), big.NewFloat(params.Ether))
}

const (
	MetadataNotFoundErr       = "metadata section not found"
	InvalidMetadataLengthErr  = "invalid metadata length"
	FailedToDecodeMetadataErr = "failed to decode metadata"
	NotCompiledWithSolcErr    = "not compiled with solc"
)

// Pragma represents the version of the Solidity compiler used to compile the contract
type Pragma struct {
	Minor uint64
	Major uint64
	Patch uint64
}

// String returns the string representation of the Pragma
func (p Pragma) String() string {
	return fmt.Sprintf("%d.%d.%d", p.Major, p.Minor, p.Patch)
}

// DecodePragmaVersion extracts the pragma version from the bytecode or returns an error if it's not found or can't be decoded.
// Based on https://www.rareskills.io/post/solidity-metadata
func DecodePragmaVersion(bytecode string) (Pragma, error) {
	metadataEndIndex := len(bytecode) - 4
	metadataLengthHex := bytecode[metadataEndIndex:]
	metadataLengthByte, err := hex.DecodeString(metadataLengthHex)

	if err != nil {
		return Pragma{}, fmt.Errorf("failed to decode metadata length: %v", err)
	}

	metadataByteLengthUint, err := strconv.ParseUint(hex.EncodeToString(metadataLengthByte), 16, 16)
	if err != nil {
		return Pragma{}, fmt.Errorf("failed to convert metadata length to int: %v", err)
	}

	// each byte is represented by 2 characters in hex
	metadataLengthInt := int(metadataByteLengthUint) * 2

	// if we get nonsensical metadata length, it means that metadata section is not present and last 2 bytes do not represent metadata length
	if metadataLengthInt > len(bytecode) {
		return Pragma{}, errors.New(MetadataNotFoundErr)
	}

	metadataStarIndex := metadataEndIndex - metadataLengthInt
	maybeMetadata := bytecode[metadataStarIndex:metadataEndIndex]

	if len(maybeMetadata) != metadataLengthInt {
		return Pragma{}, fmt.Errorf("%s. expected: %d, actual: %d", InvalidMetadataLengthErr, metadataLengthInt, len(maybeMetadata))
	}

	// INVALID opcode is used as a marker for the start of the metadata section
	metadataMarker := "fe"
	maybeMarker := bytecode[metadataStarIndex-2 : metadataStarIndex]

	if maybeMarker != metadataMarker {
		return Pragma{}, errors.New(MetadataNotFoundErr)
	}

	// this is byte-encoded version of the string "solc"
	solcMarker := "736f6c63"
	if !strings.Contains(maybeMetadata, solcMarker) {
		return Pragma{}, errors.New(NotCompiledWithSolcErr)
	}

	// now that we know that last section indeed contains metadata let's grab the version
	maybePragma := bytecode[metadataEndIndex-6 : metadataEndIndex]
	majorHex := maybePragma[0:2]
	minorHex := maybePragma[2:4]
	patchHex := maybePragma[4:6]

	major, err := strconv.ParseUint(majorHex, 16, 16)
	if err != nil {
		return Pragma{}, fmt.Errorf("%s: %v", FailedToDecodeMetadataErr, err)
	}

	minor, err := strconv.ParseUint(minorHex, 16, 16)
	if err != nil {
		return Pragma{}, fmt.Errorf("%s: %v", FailedToDecodeMetadataErr, err)
	}

	patch, err := strconv.ParseUint(patchHex, 16, 16)
	if err != nil {
		return Pragma{}, fmt.Errorf("%s: %v", FailedToDecodeMetadataErr, err)
	}

	return Pragma{Major: major, Minor: minor, Patch: patch}, nil
}

// DoesPragmaSupportCustomRevert checks if the pragma version supports custom revert messages (must be >= 0.8.4)
func DoesPragmaSupportCustomRevert(pragma Pragma) bool {
	return pragma.Minor > 8 || (pragma.Minor == 8 && pragma.Patch >= 4) || pragma.Major > 0
}

func wrapErrInMessageWithASuggestion(err error) error {
	message := `

This error could be caused by several issues. Please try these steps to resolve it:

1. Make sure the address you are using has sufficient funds.
2. Use a different RPC node. The current one might be out of sync or malfunctioning.
3. Review the logs to see if automatic gas estimations were unsuccessful. If they were, check that the fallback gas prices are set correctly.
4. If a gas limit was manually set, try commenting it out to let the node estimate it instead and see if that resolves the issue.
5. Conversely, if a gas limit was set manually, try increasing it to a higher value. This adjustment is especially crucial for some Layer 2 solutions that have variable gas limits.

Original error:`
	return fmt.Errorf("%s\n%s", message, err.Error())
}
