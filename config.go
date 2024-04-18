package seth

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
)

const (
	ErrReadSethConfig         = "failed to read TOML config for seth"
	ErrReadKeyFileConfig      = "failed to read TOML keyfile config"
	ErrUnmarshalSethConfig    = "failed to unmarshal TOML config for seth"
	ErrUnmarshalKeyFileConfig = "failed to unmarshal TOML keyfile config for seth"
	ErrEmptyNetwork           = "no network was selected, set NETWORK=..., check TOML config for available networks and set the env var"
	ErrEmptyRootPrivateKey    = "no private keys were set, set ROOT_PRIVATE_KEY=..."

	GETH  = "Geth"
	ANVIL = "Anvil"
)

type Config struct {
	// ephemeral is internal option used only from code
	ephemeral          bool
	EphemeralAddrs     *int64   `toml:"ephemeral_addresses_number"`
	RootKeyFundsBuffer *big.Int `toml:"root_key_funds_buffer"`

	ABIDir                        string `toml:"abi_dir"`
	BINDir                        string `toml:"bin_dir"`
	ContractMapFile               string `toml:"contract_map_file"`
	SaveDeployedContractsMap      bool   `toml:"save_deployed_contracts_map"`
	KeyFilePath                   string
	Network                       *Network         `toml:"network"`
	Networks                      []*Network       `toml:"networks"`
	NonceManager                  *NonceManagerCfg `toml:"nonce_manager"`
	TracingLevel                  string           `toml:"tracing_level"`
	TraceToJson                   bool             `toml:"trace_to_json"`
	PendingNonceProtectionEnabled bool             `toml:"pending_nonce_protection_enabled"`
	// internal fields
	ConfigDir                string `toml:"abs_path"`
	RevertedTransactionsFile string

	ExperimentsEnabled []string `toml:"experiments_enabled"`
}

type NonceManagerCfg struct {
	KeySyncRateLimitSec int       `toml:"key_sync_rate_limit_per_sec"`
	KeySyncTimeout      *Duration `toml:"key_sync_timeout"`
	KeySyncRetries      uint      `toml:"key_sync_retries"`
	KeySyncRetryDelay   *Duration `toml:"key_sync_retry_delay"`
}

type Network struct {
	Name                         string    `toml:"name"`
	ChainID                      string    `toml:"chain_id"`
	URLs                         []string  `toml:"urls_secret"`
	EIP1559DynamicFees           bool      `toml:"eip_1559_dynamic_fees"`
	GasPrice                     int64     `toml:"gas_price"`
	GasFeeCap                    int64     `toml:"gas_fee_cap"`
	GasTipCap                    int64     `toml:"gas_tip_cap"`
	GasLimit                     uint64    `toml:"gas_limit"`
	TxnTimeout                   *Duration `toml:"transaction_timeout"`
	TransferGasFee               int64     `toml:"transfer_gas_fee"`
	PrivateKeys                  []string  `toml:"private_keys_secret"`
	GasPriceEstimationEnabled    bool      `toml:"gas_price_estimation_enabled"`
	GasPriceEstimationBlocks     uint64    `toml:"gas_price_estimation_blocks"`
	GasPriceEstimationTxPriority string    `toml:"gas_price_estimation_tx_priority"`
}

// ReadConfig reads the TOML config file from location specified by env var "SETH_CONFIG_PATH" and returns a Config struct
func ReadConfig() (*Config, error) {
	cfgPath := os.Getenv("SETH_CONFIG_PATH")
	if cfgPath == "" {
		return nil, errors.New(ErrEmptyConfigPath)
	}
	var cfg *Config
	d, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, errors.Wrap(err, ErrReadSethConfig)
	}
	err = toml.Unmarshal(d, &cfg)
	if err != nil {
		return nil, errors.Wrap(err, ErrUnmarshalSethConfig)
	}
	absPath, err := filepath.Abs(cfgPath)
	if err != nil {
		return nil, err
	}
	cfg.ConfigDir = filepath.Dir(absPath)
	snet := os.Getenv("NETWORK")
	if snet == "" {
		return nil, errors.New(ErrEmptyNetwork)
	}
	for _, n := range cfg.Networks {
		if n.Name == snet {
			cfg.Network = n
		}
	}
	if cfg.Network == nil {
		return nil, fmt.Errorf("network %s not found", snet)
	}

	rootPrivateKey := os.Getenv("ROOT_PRIVATE_KEY")
	if rootPrivateKey == "" {
		return nil, errors.New(ErrEmptyRootPrivateKey)
	} else {
		cfg.Network.PrivateKeys = append(cfg.Network.PrivateKeys, rootPrivateKey)
	}
	L.Trace().Interface("Config", cfg).Msg("Parsed seth config")
	return cfg, nil
}

// ParseKeys parses private keys from the config
func (c *Config) ParseKeys() ([]common.Address, []*ecdsa.PrivateKey, error) {
	addresses := make([]common.Address, 0)
	privKeys := make([]*ecdsa.PrivateKey, 0)
	for _, k := range c.Network.PrivateKeys {
		privateKey, err := crypto.HexToECDSA(k)
		if err != nil {
			return nil, nil, err
		}
		publicKey := privateKey.Public()
		publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
		if !ok {
			return nil, nil, err
		}
		pubKeyAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
		addresses = append(addresses, pubKeyAddress)
		privKeys = append(privKeys, privateKey)
	}
	return addresses, privKeys, nil
}

// IsSimulatedNetwork returns true if the network is simulated (i.e. Geth or Anvil)
func (c *Config) IsSimulatedNetwork() bool {
	networkName := strings.ToLower(c.Network.Name)
	return networkName == strings.ToLower(GETH) || networkName == strings.ToLower(ANVIL)
}

// GenerateContractMapFileName generates a file name for the contract map
func (c *Config) GenerateContractMapFileName() string {
	networkName := strings.ToLower(c.Network.Name)
	now := time.Now().Format("2006-01-02-15-04-05")
	return fmt.Sprintf(ContractMapFilePattern, networkName, now)
}

// ShoulSaveDeployedContractMap returns true if the contract map should be saved (i.e. not a simulated network and functionality is enabled)
func (c *Config) ShoulSaveDeployedContractMap() bool {
	return !c.IsSimulatedNetwork() && c.SaveDeployedContractsMap
}

func readKeyFileConfig(cfg *Config) error {
	cfg.KeyFilePath = os.Getenv("SETH_KEYFILE_PATH")
	if cfg.KeyFilePath != "" {
		if cfg.EphemeralAddrs != nil && *cfg.EphemeralAddrs != 0 {
			return fmt.Errorf("SETH_KEYFILE_PATH environment variable is set to '%s' and ephemeral addresses are enabled, please disable ephemeral addresses or remove the keyfile path from the environment variable. You cannot use both modes at the same time", cfg.KeyFilePath)
		}
		if _, err := os.Stat(cfg.KeyFilePath); os.IsNotExist(err) {
			return nil
		}
		var kf *KeyFile
		kfd, err := os.ReadFile(cfg.KeyFilePath)
		if err != nil {
			return errors.Wrap(err, ErrReadKeyFileConfig)
		}
		err = toml.Unmarshal(kfd, &kf)
		if err != nil {
			return errors.Wrap(err, ErrUnmarshalKeyFileConfig)
		}
		for _, pk := range kf.Keys {
			cfg.Network.PrivateKeys = append(cfg.Network.PrivateKeys, pk.PrivateKey)
		}
	}
	return nil
}

func (c *Config) setEphemeralAddrs() {
	if c.EphemeralAddrs == nil {
		c.EphemeralAddrs = &SixtyEphemeralAddresses
	}

	if *c.EphemeralAddrs == 0 {
		c.ephemeral = false
	}

	if c.KeyFilePath == "" && *c.EphemeralAddrs != 0 {
		c.ephemeral = true
	}

	if c.RootKeyFundsBuffer == nil {
		c.RootKeyFundsBuffer = ZeroRootKeyFundsBuffer
	}
}

const (
	Experiment_SlowFundsReturn    = "slow_funds_return"
	Experiment_Eip1559FeeEqualier = "eip_1559_fee_equalizer"
)

func (c *Config) IsExperimentEnabled(experiment string) bool {
	for _, e := range c.ExperimentsEnabled {
		if e == experiment {
			return true
		}
	}
	return false
}
