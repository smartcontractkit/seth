# Seth
Reliable and debug-friendly Ethereum client

[![Decoding tests](https://github.com/smartcontractkit/seth/actions/workflows/test_decode.yml/badge.svg)](https://github.com/smartcontractkit/seth/actions/workflows/test_decode.yml)
[![Tracing tests](https://github.com/smartcontractkit/seth/actions/workflows/test_trace.yml/badge.svg)](https://github.com/smartcontractkit/seth/actions/workflows/test_trace.yml)
[![API tests](https://github.com/smartcontractkit/seth/actions/workflows/test_api.yml/badge.svg)](https://github.com/smartcontractkit/seth/actions/workflows/test_api.yml)
[![CLI tests](https://github.com/smartcontractkit/seth/actions/workflows/test_cli.yml/badge.svg)](https://github.com/smartcontractkit/seth/actions/workflows/test_cli.yml)
[![Integration tests (testnets)](https://github.com/smartcontractkit/seth/actions/workflows/test_decode_testnet.yml/badge.svg)](https://github.com/smartcontractkit/seth/actions/workflows/test_decode_testnet.yml)
<br/>

## Goals
- Be a thin, debuggable and battle tested wrapper on top of `go-ethereum`
- Decode all transaction inputs/outputs/logs for all ABIs you are working with, automatically
- Simple synchronous API
- Do not handle `nonces` on the client side, trust the server
- Do not wrap `bind` generated contracts, small set of additional debug API
- Resilient: should execute transactions even if there is a gas spike or an RPC outage (failover)
- Well tested: should provide a suite of e2e tests that can be run on testnets to check integration

## Examples
Check [examples](./examples) folder

Lib is providing a small amount of helpers for decoding handling that you can use with vanilla `go-ethereum` generated wrappers
```
// Decode waits for transaction and decode all the data/errors
Decode(tx *types.Transaction, txErr error) (*DecodedTransaction, error)

// NewTXOpts returns a new sequential transaction options wrapper,
// sets opts.GasPrice and opts.GasLimit from seth.toml or override with options
NewTXOpts(o ...TransactOpt) *bind.TransactOpts

// NewCallOpts returns a new call options wrapper
NewCallOpts(o ...CallOpt) *bind.CallOpts
```

By default, we are using the `root` key `0`, but you can also use `keys` from `keyfile.toml`
```
// NewCallKeyOpts returns a new sequential call options wrapper from the key N
NewCallKeyOpts(keyNum int, o ...CallOpt) *bind.CallOpts

// NewTXKeyOpts returns a new transaction options wrapper called from the key N
NewTXKeyOpts(keyNum int, o ...TransactOpt) *bind.TransactOpts
```

Start `Geth` in a separate terminal, then run the examples
```
make GethSync
cd examples
go test -v
```

## Setup
We are using [nix](https://nixos.org/)

Enter the shell
```
nix develop
```

## Building contracts
We have `go-ethereum` and [foundry](https://github.com/foundry-rs/foundry) tools inside `nix` shell
```
make build
```

## Testing
To run tests on a local network, first start it
```
make AnvilSync
```
Or use latest `Geth`
```
make GethSync
```

You can use default `hardhat` key `ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80` to run tests

Run the [decode](./client_decode_test.go) tests
```
make network=Anvil root_private_key=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 test
make network=Geth root_private_key=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 test
```
Check other params in [seth.toml](./seth.toml), select any network and use your key for testnets

User facing API tests are [here](./client_api_test.go)
```
make network=Anvil root_private_key=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 test_api
make network=Geth root_private_key=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 test_api
```

CLI tests
```
make network=Anvil root_private_key=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 test_cli
make network=Geth root_private_key=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 test_cli
```

Tracing tests
```
make network=Anvil root_private_key=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 test_trace
make network=Geth root_private_key=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 test_trace
```

# Config
### env vars
Some crucial data is stored in env vars, create `.envrc` and use `source .envrc`, or use `direnv`
```
export SETH_LOG_LEVEL=info # global logger level
export SETH_CONFIG_PATH=seth.toml # path to the toml config
export SETH_KEYFILE_PATH=keyfile.toml # keyfile path for using multiple keys
export NETWORK=Geth # selected network
export ROOT_PRIVATE_KEY=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 # root private key

alias seth="go run cmd/seth/seth.go" # useful alias for keyfile CLI
```

If `SETH_KEYFILE_PATH` is not set then client will create 60 ephemeral keys and won't return any funds

Use `SETH_KEYFILE_PATH` for testnets/mainnets and `ephemeral` mode only when testing against simulated network
### seth.toml
Set up your ABI directory (relative to `seth.toml`)
```
abi_dir = "contracts/abi"
```

Setup your BIN directory (relative to `seth.toml`)
```
bin_dir = "contracts/bin"
```

You can enable auto-tracing for all transactions, which means that every time you use `Decode()` we will not only decode the transaction but also trace all calls made within the transaction, together with all inputs, outputs, logs and events
```
tracing_enabled = true
```

Additionally you can also enable saving all tracing information to JSON files with:
```
trace_to_json = true
```

You can add more networks like this:
```
[[Networks]]
name = "Fuji"
chain_id = "43113"
transaction_timeout = "30s"
# legacy transactions
gas_price = 1_000_000_000
# EIP-1559 transactions
eip_1559_dynamic_fees = true
gas_fee_cap = 25_000_000_000
gas_tip_cap = 1_800_000_000
urls_secret = ["..."]
```

If you want to save addresses of deployed contracts, you can enable it with:
```
save_deployed_contracts_map = true
```


If you want to re-use previously deployed contracts you can indicate file name in `seth.toml`:
```
contract_map_file = "deployed_contracts_mumbai.toml"
```
Both features only works for live networks, otherwise they are ignored and nothing is saved/read from for simulated networks.

## CLI

### Multiple keys manipulation (keyfile.toml)
To use multiple keys in your tests you can create a `keyfile.toml` using CLI

Set up the alias, see `.envrc` configuration above
```
alias seth="go run cmd/seth/seth.go"
```

Create a new `keyfile` with 10 new accounts funded from the root key (KEYS env var)
```
seth -n Geth keys split -a 10
```
Run the tests, then return funds back, when needed
```
seth -n Geth keys return
```
Update the balances
```
seth -n Geth keys update
```
Remove the `keyfile`
```
seth -n Geth keys remove
```
### Manual gas price estimation
In order to adjust gas price for a transaction, you can use `seth gas` command
```
seth -n Fuji gas -b 10000 -tp 0.99
```
This will analyze last 10k blocks and give you 25/50/75/99th/Max percentiles for base fees and tip fees

`-tp 0.99` requests the 99th tip percentile across all the transaction in one block and calculates 25/50/75/99th/Max across all blocks

### Bulk tracing
You can trace multiple transactions at once using `seth trace` command. Example:
```
SETH_CONFIG_PATH=seth.toml go run cmd/seth/seth.go -n=Geth trace -f reverted_transactions.json
```

You need to pass a file with a list of transaction hashes to trace. The file should be a JSON array of transaction hashes, like this:
```
[
  "0x...",
  "0x...",
  "0x...",
  ...
]
```

(Note that currently Seth automatically creates `reverted_transactions_<network>_<date>.json` with all reverted transactions, so you can use this file as input for the `trace` command.)


## Features
- [x] Decode named inputs
- [x] Decode named outputs
- [x] Decode anonymous outputs
- [x] Decode logs
- [x] Decode indexed logs
- [x] Decode old string reverts
- [x] Decode new typed reverts
- [x] EIP-1559 support
- [x] Multi-keys client support
- [x] CLI to manipulate test keys
- [x] Simple manual gas price estimation
- [ ] Tuned gas prices for testnets (optimized for fast transaction times)
- [ ] Fail over client logic
- [ ] Decode collided event hashes
- [x] Tracing support (4byte)
- [x] Tracing support (callTracer)
- [ ] Tracing support (prestate)
- [x] Tracing decoding
- [x] Tracing tests
- [ ] More tests for corner cases of decoding/tracing
- [x] Saving of deployed contracts mapping (`address -> ABI_name`) for live networks
- [x] Reading of deployed contracts mappings for live networks

You can read more about how ABI finding and contract map works [here](./docs/abi_finder_contract_map.md) and about contract store here [here](./docs/contract_store.md).