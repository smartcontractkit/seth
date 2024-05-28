## Running multi-key load test with Seth and WASP
To effectively simulate transaction workloads from multiple keys, you can utilize a "rotating wallet." Refer to the [example](client_wasp_test.go) code provided for guidance.

There are 2 modes: Ephemeral and a static keyfile mode

### Ephemeral mode
We generate 60 ephemeral keys and run the test, set `ephemeral_addresses_number` in `seth.toml`

This mode **should never be used on testnets or mainnets** in order not to lose funds. Please use it to test with simulated networks, like private `Geth` or `Anvil`

```toml
ephemeral_addresses_number = 60
```
Then start the Geth and run the test

```
nix develop
make GethSync

// another terminal, from examples_wasp dir
export SETH_LOG_LEVEL=debug
export SETH_CONFIG_PATH=seth.toml
export SETH_NETWORK=Geth
export SETH_ROOT_PRIVATE_KEY=ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80
export LOKI_TENANT_ID=promtail
export LOKI_URL=...

go test -v -run TestWithWasp
```
See both [generator](client_wasp_test.go) and [test](client_wasp_test.go) implementation example

Check your results [here](https://grafana.ops.prod.cldev.sh/d/WaspDebug/waspdebug?orgId=1&from=now-5m&to=now)

If you see `key sync timeout`, just increase `ephemeral_addresses_number` to have more load

You can also change default `key_sync` values
```toml
[nonce_manager]
# 20 req/s limit for key syncing
key_sync_rate_limit_per_sec = 20
# key synchronization timeout, if it's more than N sec you'll see an error, raise amount of keys or increase the timeout
key_sync_timeout = "30s"
# key sync retry delay, each N seconds we'll updage each key nonce
key_sync_retry_delay = "1s"
# total number of retries until we throw an error
key_sync_retries = 30
```