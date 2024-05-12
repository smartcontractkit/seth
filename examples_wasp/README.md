## Running multi-key load test with Seth and WASP
To effectively simulate transaction workloads from multiple keys, you can utilize a "rotating wallet." Refer to the [example](client_wasp_test.go) code provided for guidance.

There are 2 modes: Ephemeral and a static keyfile mode

### Ephemeral mode
We generate 60 ephemeral keys and run the test, set `ephemeral_addresses_number` in `seth.toml`

```toml
ephemeral_addresses_number = 60
```
Then start the Geth and run the test

```
make GethSync
go test -v -run TestWithWasp
```

Check your results [here](https://grafana.ops.prod.cldev.sh/d/WaspDebug/waspdebug?orgId=1&from=now-5m&to=now)