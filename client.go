package seth

import (
	"context"
	"crypto/ecdsa"
	verr "errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/ethereum/go-ethereum"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

const (
	ErrEmptyConfigPath    = "toml config path is empty, set SETH_CONFIG_PATH"
	ErrCreateABIStore     = "failed to create ABI store"
	ErrReadingKeys        = "failed to read keys"
	ErrCreateNonceManager = "failed to create nonce manager"
	ErrCreateTracer       = "failed to create tracer"
	ErrReadContractMap    = "failed to read deployed contract map"
	ErrNoKeyLoaded        = "failed to load private key"

	ContractMapFilePattern = "deployed_contracts_%s_%s.toml"
)

var (
	// DefaultEphemeralAddresses is amount of addresses created in ephemeral client mode
	DefaultEphemeralAddresses int64 = 60
)

// Client is a vanilla go-ethereum client with enhanced debug logging
type Client struct {
	Cfg                      *Config
	Client                   *ethclient.Client
	Addresses                []common.Address
	PrivateKeys              []*ecdsa.PrivateKey
	ChainID                  int64
	URL                      string
	Context                  context.Context
	CancelFunc               context.CancelFunc
	Errors                   []error
	ContractStore            *ContractStore
	NonceManager             *NonceManager
	Tracer                   *Tracer
	TraceReverted            bool
	ContractAddressToNameMap ContractMap
	ABIFinder                *ABIFinder
}

// NewClientWithConfig creates a new seth client with all deps setup from config
func NewClientWithConfig(cfg *Config) (*Client, error) {
	cfg.setEphemeralAddrs()
	cs, err := NewContractStore(filepath.Join(cfg.ConfigDir, cfg.ABIDir), filepath.Join(cfg.ConfigDir, cfg.BINDir))
	if err != nil {
		return nil, errors.Wrap(err, ErrCreateABIStore)
	}
	if cfg.ephemeral {
		pkeys, err := NewEphemeralKeys(*cfg.EphemeralAddrs)
		if err != nil {
			return nil, err
		}
		cfg.Network.PrivateKeys = append(cfg.Network.PrivateKeys, pkeys...)
	}
	addrs, pkeys, err := cfg.ParseKeys()
	if err != nil {
		return nil, errors.Wrap(err, ErrReadingKeys)
	}
	nm, err := NewNonceManager(cfg, addrs, pkeys)
	if err != nil {
		return nil, errors.Wrap(err, ErrCreateNonceManager)
	}

	if !cfg.IsSimulatedNetwork() && cfg.SaveDeployedContractsMap && cfg.ContractMapFile == "" {
		cfg.ContractMapFile = cfg.GenerateContractMapFileName()
	}

	// this part is kind of duplicated in NewClientRaw, but we need to create contract map before creating Tracer
	// so that both the tracer and client have references to the same map
	contractAddressToNameMap := make(ContractMap)
	if !cfg.IsSimulatedNetwork() {
		contractAddressToNameMap, err = LoadDeployedContracts(cfg.ContractMapFile)
		if err != nil {
			return nil, errors.Wrap(err, ErrReadContractMap)
		}
	} else {
		L.Debug().Msg("Simulated network, contract map won't be read from file")
	}

	abiFinder := NewABIFinder(contractAddressToNameMap, cs)
	tr, err := NewTracer(cfg.Network.URLs[0], cs, &abiFinder, cfg, contractAddressToNameMap, addrs)
	if err != nil {
		return nil, errors.Wrap(err, ErrCreateTracer)
	}
	return NewClientRaw(
		cfg,
		addrs,
		pkeys,
		WithContractStore(cs),
		WithNonceManager(nm),
		WithTracer(tr),
		WithContractMap(contractAddressToNameMap),
		WithABIFinder(&abiFinder),
	)
}

// NewClient creates a new raw seth client with all deps setup from env vars
func NewClient() (*Client, error) {
	initDefaultLogging()
	cfg, err := ReadConfig()
	if err != nil {
		return nil, err
	}
	return NewClientWithConfig(cfg)
}

// NewClientRaw creates a new raw seth client without dependencies
func NewClientRaw(
	cfg *Config,
	addrs []common.Address,
	pkeys []*ecdsa.PrivateKey,
	opts ...ClientOpt,
) (*Client, error) {
	client, err := ethclient.Dial(cfg.Network.URLs[0])
	if err != nil {
		return nil, fmt.Errorf("failed to connect to '%s' due to: %w", cfg.Network.URLs[0], err)
	}
	cID, err := strconv.Atoi(cfg.Network.ChainID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		Cfg:         cfg,
		Client:      client,
		Addresses:   addrs,
		PrivateKeys: pkeys,
		URL:         cfg.Network.URLs[0],
		ChainID:     int64(cID),
		Context:     ctx,
		CancelFunc:  cancel,
	}
	for _, o := range opts {
		o(c)
	}

	if c.ContractAddressToNameMap == nil {
		if !cfg.IsSimulatedNetwork() {
			c.ContractAddressToNameMap, err = LoadDeployedContracts(cfg.ContractMapFile)
			if err != nil {
				return nil, errors.Wrap(err, ErrReadContractMap)
			}
			if len(c.ContractAddressToNameMap) > 0 {
				L.Info().
					Int("Size", len(c.ContractAddressToNameMap)).
					Str("File name", cfg.ContractMapFile).
					Msg("No contract map provided, read it from file")
			} else {
				L.Info().
					Msg("No contract map provided and no file found, created new one")
			}
		} else {
			L.Debug().Msg("Simulated network, contract map won't be read from file")
			c.ContractAddressToNameMap = make(ContractMap)
			L.Info().
				Msg("No contract map provided and no file found, created new one")
		}
	} else {
		L.Info().
			Int("Size", len(c.ContractAddressToNameMap)).
			Msg("Contract map was provided")
	}
	if c.NonceManager != nil {
		c.NonceManager.Client = c
		if err := c.NonceManager.UpdateNonces(); err != nil {
			return nil, err
		}
	}

	cfg.setEphemeralAddrs()

	L.Info().
		Str("NetworkName", cfg.Network.Name).
		Interface("Addresses", addrs).
		Str("RPC", cfg.Network.URLs[0]).
		Str("ChainID", cfg.Network.ChainID).
		Int64("Ephemeral keys", *cfg.EphemeralAddrs).
		Msg("Created new client")

	if cfg.ephemeral {
		bd, err := c.CalculateSubKeyFunding(*cfg.EphemeralAddrs)
		if err != nil {
			return nil, err
		}
		L.Warn().Msg("Ephemeral mode, all funds will be lost!")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		eg, egCtx := errgroup.WithContext(ctx)
		// root key is element 0 in ephemeral
		for _, addr := range c.Addresses[1:] {
			addr := addr
			eg.Go(func() error {
				return c.TransferETHFromKey(egCtx, 0, addr.Hex(), bd.AddrFunding)
			})
		}
		if err := eg.Wait(); err != nil {
			return nil, err
		}
	}

	if c.Cfg.TracingEnabled && c.Tracer == nil {
		if c.ContractStore == nil {
			cs, err := NewContractStore(filepath.Join(cfg.ConfigDir, cfg.ABIDir), filepath.Join(cfg.ConfigDir, cfg.BINDir))
			if err != nil {
				return nil, errors.Wrap(err, ErrCreateABIStore)
			}
			c.ContractStore = cs
		}
		if c.ABIFinder == nil {
			abiFinder := NewABIFinder(c.ContractAddressToNameMap, c.ContractStore)
			c.ABIFinder = &abiFinder
		}
		tr, err := NewTracer(cfg.Network.URLs[0], c.ContractStore, c.ABIFinder, cfg, c.ContractAddressToNameMap, addrs)
		if err != nil {
			return nil, errors.Wrap(err, ErrCreateTracer)
		}

		c.Tracer = tr
	}

	return c, nil
}

// Decode waits for transaction to be minted and decodes basic info of the first method that was
// called by the transaction (such as: method name, inputs and any logs/events emitted by the transaction.
// Additionally if `tracing_enabled` flag is set in Client config it will trace all other calls that were
// made during the transaction execution and print them to console using logger.
// If transaction was reverted, then tracing won't take place, but the error return will contain
// revert reason (if we were able to get it from the receipt).
func (m *Client) Decode(tx *types.Transaction, txErr error) (*DecodedTransaction, error) {
	if len(m.Errors) > 0 {
		return nil, verr.Join(m.Errors...)
	}
	if txErr != nil {
		return nil, txErr
	}
	if tx == nil {
		return nil, nil
	}
	l := L.With().Str("Transaction", tx.Hash().Hex()).Logger()
	receipt, err := m.WaitMined(context.Background(), l, m.Client, tx)
	if err != nil {
		return &DecodedTransaction{}, err
	}
	if receipt.Status == 0 {
		if err := m.callAndGetRevertReason(tx, receipt); err != nil {
			return &DecodedTransaction{}, err
		}
	}

	decoded, decodeErr := m.decodeTransaction(l, tx, receipt)
	if decodeErr != nil {
		// return error only if tracing is not enabled, as we might still get some useful data from tracing
		if !m.Cfg.TracingEnabled && strings.Contains(decodeErr.Error(), ErrNoABIMethod) {
			return nil, decodeErr
		}
	}

	if m.Cfg.TracingEnabled {
		traceErr := m.Tracer.TraceGethTX(decoded.Hash)
		if traceErr != nil {
			return nil, traceErr
		}
	}

	return decoded, nil
}

func (m *Client) TransferETHFromKey(ctx context.Context, fromKeyNum int, to string, value *big.Int) error {
	if fromKeyNum > len(m.PrivateKeys) || fromKeyNum > len(m.Addresses) {
		return errors.Wrap(errors.New(ErrNoKeyLoaded), fmt.Sprintf("requested key: %d", fromKeyNum))
	}
	toAddr := common.HexToAddress(to)
	chainID, err := m.Client.NetworkID(context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to get network ID")
	}
	rawTx := &types.LegacyTx{
		Nonce:    m.NonceManager.NextNonce(m.Addresses[fromKeyNum]).Uint64(),
		To:       &toAddr,
		Value:    value,
		Gas:      uint64(m.Cfg.Network.TransferGasFee),
		GasPrice: big.NewInt(m.Cfg.Network.GasPrice),
	}
	L.Debug().Interface("TransferTx", rawTx).Send()
	signedTx, err := types.SignNewTx(m.PrivateKeys[fromKeyNum], types.NewEIP155Signer(chainID), rawTx)
	if err != nil {
		return errors.Wrap(err, "failed to sign tx")
	}

	ctx, cancel := context.WithTimeout(ctx, m.Cfg.Network.TxnTimeout.Duration())
	defer cancel()
	err = m.Client.SendTransaction(ctx, signedTx)
	if err != nil {
		return errors.Wrap(err, "failed to send transaction")
	}
	l := L.With().Str("Transaction", signedTx.Hash().Hex()).Logger()
	l.Info().
		Int("FromKeyNum", fromKeyNum).
		Str("To", to).
		Interface("Value", value).
		Msg("Send ETH")
	_, err = m.WaitMined(ctx, l, m.Client, signedTx)
	if err != nil {
		return err
	}
	return err
}

// WaitMined the same as bind.WaitMined, awaits transaction receipt until timeout
func (m *Client) WaitMined(ctx context.Context, l zerolog.Logger, b bind.DeployBackend, tx *types.Transaction) (*types.Receipt, error) {
	queryTicker := time.NewTicker(time.Second)
	defer queryTicker.Stop()
	ctx, cancel := context.WithTimeout(ctx, m.Cfg.Network.TxnTimeout.Duration())
	defer cancel()
	for {
		receipt, err := b.TransactionReceipt(ctx, tx.Hash())
		if err == nil {
			l.Info().Int64("BlockNumber", receipt.BlockNumber.Int64()).Msg("Transaction accepted")
			return receipt, nil
		}
		if errors.Is(err, ethereum.NotFound) {
			l.Debug().Msg("Awaiting transaction")
		} else {
			l.Warn().Err(err).Msg("Failed to get receipt")
		}
		select {
		case <-ctx.Done():
			l.Error().Err(err).Msg("Transaction context is done")
			return nil, ctx.Err()
		case <-queryTicker.C:
		}
	}
}

/* ClientOpts client functional options */

// ClientOpt is a client functional option
type ClientOpt func(c *Client)

// WithContractStore ContractStore functional option
func WithContractStore(as *ContractStore) ClientOpt {
	return func(c *Client) {
		c.ContractStore = as
	}
}

// WithContractMap contractAddressToNameMap functional option
func WithContractMap(contractAddressToNameMap map[string]string) ClientOpt {
	return func(c *Client) {
		c.ContractAddressToNameMap = contractAddressToNameMap
	}
}

// WithABIFinder ABIFinder functional option
func WithABIFinder(abiFinder *ABIFinder) ClientOpt {
	return func(c *Client) {
		c.ABIFinder = abiFinder
	}
}

// WithNonceManager NonceManager functional option
func WithNonceManager(nm *NonceManager) ClientOpt {
	return func(c *Client) {
		c.NonceManager = nm
	}
}

// WithTracer Tracer functional option
func WithTracer(t *Tracer) ClientOpt {
	return func(c *Client) {
		c.Tracer = t
	}
}

/* CallOpts function options */

// CallOpt is a functional option for bind.CallOpts
type CallOpt func(o *bind.CallOpts)

// WithPending sets pending option for bind.CallOpts
func WithPending(pending bool) CallOpt {
	return func(o *bind.CallOpts) {
		o.Pending = pending
	}
}

// WithBlockNumber sets blockNumber option for bind.CallOpts
func WithBlockNumber(bn uint64) CallOpt {
	return func(o *bind.CallOpts) {
		o.BlockNumber = big.NewInt(int64(bn))
	}
}

// NewCallOpts returns a new sequential call options wrapper
func (m *Client) NewCallOpts(o ...CallOpt) *bind.CallOpts {
	co := &bind.CallOpts{
		Pending: false,
		From:    m.Addresses[0],
	}
	for _, f := range o {
		f(co)
	}
	return co
}

// NewCallKeyOpts returns a new sequential call options wrapper from the key N
func (m *Client) NewCallKeyOpts(keyNum int, o ...CallOpt) *bind.CallOpts {
	co := &bind.CallOpts{
		Pending: false,
		From:    m.Addresses[keyNum],
	}
	for _, f := range o {
		f(co)
	}
	return co
}

// TransactOpt is a wrapper for bind.TransactOpts
type TransactOpt func(o *bind.TransactOpts)

// WithValue sets value option for bind.TransactOpts
func WithValue(value *big.Int) TransactOpt {
	return func(o *bind.TransactOpts) {
		o.Value = value
	}
}

// WithGasPrice sets gasPrice option for bind.TransactOpts
func WithGasPrice(gasPrice *big.Int) TransactOpt {
	return func(o *bind.TransactOpts) {
		o.GasPrice = gasPrice
	}
}

// WithGasLimit sets gasLimit option for bind.TransactOpts
func WithGasLimit(gasLimit uint64) TransactOpt {
	return func(o *bind.TransactOpts) {
		o.GasLimit = gasLimit
	}
}

// WithNoSend sets noSend option for bind.TransactOpts
func WithNoSend(noSend bool) TransactOpt {
	return func(o *bind.TransactOpts) {
		o.NoSend = noSend
	}
}

// WithNonce sets nonce option for bind.TransactOpts
func WithNonce(nonce *big.Int) TransactOpt {
	return func(o *bind.TransactOpts) {
		o.Nonce = nonce
	}
}

// WithGasFeeCap sets gasFeeCap option for bind.TransactOpts
func WithGasFeeCap(gasFeeCap *big.Int) TransactOpt {
	return func(o *bind.TransactOpts) {
		o.GasFeeCap = gasFeeCap
	}
}

// WithGasTipCap sets gasTipCap option for bind.TransactOpts
func WithGasTipCap(gasTipCap *big.Int) TransactOpt {
	return func(o *bind.TransactOpts) {
		o.GasTipCap = gasTipCap
	}
}

// NewTXOpts returns a new transaction options wrapper,
// sets opts.GasPrice and opts.GasLimit from seth.toml or override with options
func (m *Client) NewTXOpts(o ...TransactOpt) *bind.TransactOpts {
	opts, nonce, gasPrice, gasTipCap := m.getProposedTransactionOptions(0)
	m.configureTransactionOpts(opts, nonce, gasPrice, gasTipCap, o...)
	L.Debug().
		Interface("Nonce", opts.Nonce).
		Interface("Value", opts.Value).
		Interface("GasPrice", opts.GasPrice).
		Interface("GasFeeCap", opts.GasFeeCap).
		Interface("GasTipCap", opts.GasTipCap).
		Uint64("GasLimit", opts.GasLimit).
		Msg("New transaction options")
	return opts
}

// NewTXKeyOpts returns a new transaction options wrapper,
// sets opts.GasPrice and opts.GasLimit from seth.toml or override with options
func (m *Client) NewTXKeyOpts(keyNum int, o ...TransactOpt) *bind.TransactOpts {
	L.Debug().
		Interface("KeyNum", keyNum).
		Interface("Address", m.Addresses[keyNum]).
		Msg("Estimating transaction")
	opts, nonce, gasPrice, gasTipCap := m.getProposedTransactionOptions(keyNum)
	m.configureTransactionOpts(opts, nonce, gasPrice, gasTipCap, o...)
	L.Debug().
		Interface("KeyNum", keyNum).
		Interface("Nonce", opts.Nonce).
		Interface("Value", opts.Value).
		Interface("GasPrice", opts.GasPrice).
		Interface("GasFeeCap", opts.GasFeeCap).
		Interface("GasTipCap", opts.GasTipCap).
		Uint64("GasLimit", opts.GasLimit).
		Msg("New transaction options")
	return opts
}

// AnySyncedKey returns the first synced key
func (m *Client) AnySyncedKey() int {
	return m.NonceManager.anySyncedKey()
}

// getProposedTransactionOptions gets all the tx info that network proposed
func (m *Client) getProposedTransactionOptions(keyNum int) (*bind.TransactOpts, uint64, *big.Int, *big.Int) {
	ctx, cancel := context.WithTimeout(context.Background(), m.Cfg.Network.TxnTimeout.Duration())
	defer cancel()
	nonce, err := m.Client.PendingNonceAt(ctx, m.Addresses[keyNum])
	if err != nil {
		m.Errors = append(m.Errors, err)
		// can't return nil, otherwise RPC wrapper will panic
		return &bind.TransactOpts{}, 0, nil, nil
	}
	gasPrice, err := m.Client.SuggestGasPrice(ctx)
	if err != nil {
		m.Errors = append(m.Errors, err)
		return &bind.TransactOpts{}, 0, nil, nil
	}
	var gasTipCap *big.Int
	if m.Cfg.Network.EIP1559DynamicFees {
		gasTipCap, err = m.Client.SuggestGasTipCap(ctx)
		if err != nil {
			m.Errors = append(m.Errors, err)
			return &bind.TransactOpts{}, 0, nil, nil
		}
	}
	L.Debug().
		Interface("KeyNum", keyNum).
		Uint64("Nonce", nonce).
		Interface("GasPrice", gasPrice).
		Interface("GasTipCap", gasTipCap).
		Msg("Proposed transaction options")

	opts, err := bind.NewKeyedTransactorWithChainID(m.PrivateKeys[keyNum], big.NewInt(m.ChainID))
	if err != nil {
		m.Errors = append(m.Errors, err)
		return &bind.TransactOpts{}, 0, nil, nil
	}
	return opts, nonce, gasPrice, gasTipCap
}

// configureTransactionOpts configures transaction for legacy or type-2
func (m *Client) configureTransactionOpts(
	opts *bind.TransactOpts,
	nonce uint64,
	gasPrice *big.Int,
	gasTipCap *big.Int,
	o ...TransactOpt,
) *bind.TransactOpts {
	opts.Nonce = big.NewInt(int64(nonce))
	if m.Cfg.Network.GasPrice == 0 {
		opts.GasPrice = gasPrice
	} else {
		opts.GasPrice = big.NewInt(m.Cfg.Network.GasPrice)
	}
	opts.GasLimit = m.Cfg.Network.GasLimit

	if m.Cfg.Network.EIP1559DynamicFees {
		opts.GasPrice = nil
		opts.GasFeeCap = big.NewInt(m.Cfg.Network.GasFeeCap)
		if m.Cfg.Network.GasTipCap == 0 {
			opts.GasTipCap = gasTipCap
		} else {
			opts.GasTipCap = big.NewInt(m.Cfg.Network.GasTipCap)
		}
	}
	for _, f := range o {
		f(opts)
	}
	return opts
}

// DeployContract deploys contract using ABI and bytecode passed to it, waits for transaction to be minted and contract really
// available at the address, so that when the method returns it's safe to interact with it. It also saves the contract address and ABI name
// to the contract map, so that we can use that, when tracing transactions. It is suggested to use name identical to the name of the contract Solidity file.
func (m *Client) DeployContract(auth *bind.TransactOpts, name string, abi abi.ABI, bytecode []byte, params ...interface{}) (DeploymentData, error) {
	address, tx, contract, err := bind.DeployContract(auth, abi, bytecode, m.Client, params...)

	if err != nil {
		return DeploymentData{}, err
	}

	L.Info().
		Str("Address", address.Hex()).
		Str("TXHash", tx.Hash().Hex()).
		Msgf("Deploying %s contract", name)

	// I had this one failing sometimes, when transaction has been minted, but contract cannot be found yet at address
	if err := retry.Do(
		func() error {
			_, err := bind.WaitDeployed(context.Background(), m.Client, tx)
			return err
		}, retry.OnRetry(func(i uint, _ error) {
			L.Debug().Uint("Attempt", i).Msg("Waiting for contract to be deployed")
		}),
		retry.DelayType(retry.FixedDelay),
		retry.Attempts(10),
		retry.Delay(time.Duration(1)*time.Second),
		retry.RetryIf(func(err error) bool {
			return strings.Contains(strings.ToLower(err.Error()), "no contract code at given address") ||
				strings.Contains(strings.ToLower(err.Error()), "no contract code after deployment")
		}),
	); err != nil {
		return DeploymentData{}, err
	}

	L.Info().
		Str("Address", address.Hex()).
		Str("TXHash", tx.Hash().Hex()).
		Msgf("Deployed %s contract", name)

	m.ContractAddressToNameMap.AddContract(address.Hex(), name)

	if _, ok := m.ContractStore.GetABI(name); !ok {
		m.ContractStore.AddABI(name, abi)
	}

	if !m.Cfg.ShoulSaveDeployedContractMap() {
		return DeploymentData{Address: address, Transaction: tx, BoundContract: contract}, nil
	}

	if err := SaveDeployedContract(m.Cfg.ContractMapFile, name, address.Hex()); err != nil {
		L.Warn().
			Err(err).
			Msg("Failed to save deployed contract address to file")
	}

	return DeploymentData{Address: address, Transaction: tx, BoundContract: contract}, nil
}

type DeploymentData struct {
	Address       common.Address
	Transaction   *types.Transaction
	BoundContract *bind.BoundContract
}

// DeployContractFromContractStore deploys contract from Seth's Contract Store, waits for transaction to be minted and contract really
// available at the address, so that when the method returns it's safe to interact with it. It also saves the contract address and ABI name
// to the contract map, so that we can use that, when tracing transactions. Name by which you refer the contract should be the same as the
// name of ABI file (you can omit the .abi suffix).
func (m *Client) DeployContractFromContractStore(auth *bind.TransactOpts, name string, params ...interface{}) (DeploymentData, error) {
	if m.ContractStore == nil {
		return DeploymentData{}, errors.New("ABIStore is nil")
	}

	name = strings.TrimSuffix(name, ".abi")
	name = strings.TrimSuffix(name, ".bin")

	abi, ok := m.ContractStore.ABIs[name+".abi"]
	if !ok {
		return DeploymentData{}, errors.New("ABI not found")
	}

	bytecode, ok := m.ContractStore.BINs[name+".bin"]
	if !ok {
		return DeploymentData{}, errors.New("BIN not found")
	}

	data, err := m.DeployContract(auth, name, abi, bytecode, params...)
	if err != nil {
		return DeploymentData{}, err
	}

	return data, nil
}

func (m *Client) SaveDecodedCallsAsJson(dirname string) error {
	return m.Tracer.SaveDecodedCallsAsJson(dirname)
}

type TransactionLog struct {
	Topics []common.Hash
	Data   []byte
}

func (t TransactionLog) GetTopics() []common.Hash {
	return t.Topics
}

func (t TransactionLog) GetData() []byte {
	return t.Data
}

func (m *Client) decodeContractLogs(l zerolog.Logger, logs []types.Log, a abi.ABI) ([]DecodedTransactionLog, error) {
	l.Trace().Msg("Decoding events")
	var eventsParsed []DecodedTransactionLog
	for _, lo := range logs {
		for _, evSpec := range a.Events {
			if evSpec.ID.Hex() == lo.Topics[0].Hex() {
				d := TransactionLog{lo.Topics, lo.Data}
				l.Trace().Str("Name", evSpec.RawName).Str("Signature", evSpec.Sig).Msg("Unpacking event")
				eventsMap, topicsMap, err := decodeEventFromLog(l, a, evSpec, d)
				if err != nil {
					return nil, errors.Wrap(err, ErrDecodeLog)
				}
				parsedEvent := decodedLogFromMaps(&DecodedTransactionLog{}, eventsMap, topicsMap)
				if decodedTransactionLog, ok := parsedEvent.(*DecodedTransactionLog); ok {
					decodedTransactionLog.Signature = evSpec.Sig
					m.mergeLogMeta(decodedTransactionLog, lo)
					eventsParsed = append(eventsParsed, *decodedTransactionLog)
					l.Trace().Interface("Log", parsedEvent).Msg("Transaction log")
				} else {
					l.Trace().
						Str("Actual type", fmt.Sprintf("%T", decodedTransactionLog)).
						Msg("Failed to cast decoded event to DecodedCommonLog")
				}
			}
		}
	}
	return eventsParsed, nil
}

// mergeLogMeta add metadata from log
func (m *Client) mergeLogMeta(pe *DecodedTransactionLog, l types.Log) {
	pe.Address = l.Address
	pe.Topics = make([]string, 0)
	for _, t := range l.Topics {
		pe.Topics = append(pe.Topics, t.String())
	}
	pe.BlockNumber = l.BlockNumber
	pe.Index = l.Index
	pe.TXHash = l.TxHash.Hex()
	pe.TXIndex = l.TxIndex
	pe.Removed = l.Removed
}
