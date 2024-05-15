package seth

import (
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/seth"
	"github.com/urfave/cli/v2"
	"os"
	"path/filepath"
)

const (
	ErrNoNetwork = "no network specified, use -n flag. Ex.: seth -n Geth keys update"
)

var C *seth.Client

func RunCLI(args []string) error {
	app := &cli.App{
		Name:      "seth",
		Version:   "v1.0.0",
		Usage:     "seth CLI",
		UsageText: `utility to create and control Ethereum keys and give you more debug info about chains`,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "networkName", Aliases: []string{"n"}},
		},
		Before: func(cCtx *cli.Context) error {
			networkName := cCtx.String("networkName")
			if networkName == "" {
				return errors.New(ErrNoNetwork)
			}
			_ = os.Setenv("NETWORK", networkName)
			if cCtx.Args().Len() > 0 && cCtx.Args().First() != "trace" {
				var err error
				switch cCtx.Args().First() {
				case "keys":
					var cfg *seth.Config
					cfg, err = seth.ReadConfig()
					if err != nil {
						return err
					}
					keyfilePath := os.Getenv(seth.KEYFILE_PATH_ENV_VAR)
					if keyfilePath == "" {
						return errors.New("No keyfile path specified in KEYFILE_PATH_ENV_VAR env var")
					}
					cfg.KeyFileSource = seth.KeyFileSourceFile
					cfg.KeyFilePath = keyfilePath
					C, err = seth.NewClientWithConfig(cfg)
					if err != nil {
						return err
					}
				case "gas":
					var cfg *seth.Config
					var pk string
					_, pk, err = seth.NewAddress()
					if err != nil {
						return err
					}

					err = os.Setenv("ROOT_PRIVATE_KEY", pk)
					if err != nil {
						return err
					}

					cfg, err = seth.ReadConfig()
					if err != nil {
						return err
					}
					C, err = seth.NewClientWithConfig(cfg)
					if err != nil {
						return err
					}
				case "trace":
					return nil
				}
				if err != nil {
					return err
				}
			}
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:        "gas",
				HelpName:    "gas",
				Aliases:     []string{"g"},
				Description: "get various info about gas prices",
				Flags: []cli.Flag{
					&cli.Int64Flag{Name: "blocks", Aliases: []string{"b"}},
					&cli.Float64Flag{Name: "tipPercentile", Aliases: []string{"tp"}},
				},
				Action: func(cCtx *cli.Context) error {
					ge := seth.NewGasEstimator(C)
					blocks := cCtx.Uint64("blocks")
					tipPerc := cCtx.Float64("tipPercentile")
					stats, err := ge.Stats(blocks, tipPerc)
					if err != nil {
						return err
					}
					seth.L.Info().
						Interface("Max", stats.GasPrice.Max).
						Interface("99", stats.GasPrice.Perc99).
						Interface("75", stats.GasPrice.Perc75).
						Interface("50", stats.GasPrice.Perc50).
						Interface("25", stats.GasPrice.Perc25).
						Msg("Base fee (Wei)")
					seth.L.Info().
						Interface("Max", stats.TipCap.Max).
						Interface("99", stats.TipCap.Perc99).
						Interface("75", stats.TipCap.Perc75).
						Interface("50", stats.TipCap.Perc50).
						Interface("25", stats.TipCap.Perc25).
						Msg("Priority fee (Wei)")
					seth.L.Info().
						Interface("GasPrice", stats.SuggestedGasPrice).
						Msg("Suggested gas price now")
					seth.L.Info().
						Interface("GasTipCap", stats.SuggestedGasTipCap).
						Msg("Suggested gas tip cap now")

					type asTomlCfg struct {
						GasPrice int64 `toml:"gas_price"`
						GasTip   int64 `toml:"gas_tip_cap"`
						GasFee   int64 `toml:"gas_fee_cap"`
					}

					tomlCfg := asTomlCfg{
						GasPrice: stats.SuggestedGasPrice.Int64(),
						GasTip:   stats.SuggestedGasTipCap.Int64(),
						GasFee:   stats.SuggestedGasPrice.Int64() + stats.SuggestedGasTipCap.Int64(),
					}

					marshalled, err := toml.Marshal(tomlCfg)
					if err != nil {
						return err
					}

					seth.L.Info().Msgf("Fallback prices for TOML config:\n%s", string(marshalled))

					return err
				},
			},
			{
				Name:        "keys",
				HelpName:    "keys",
				Aliases:     []string{"k"},
				Description: "key management commands",
				ArgsUsage:   "",
				Subcommands: []*cli.Command{
					{
						Name:        "update",
						HelpName:    "update",
						Aliases:     []string{"u"},
						Description: "update balances for all the keys in keyfile.toml",
						ArgsUsage:   "seth keys update",
						Action: func(cCtx *cli.Context) error {
							return seth.UpdateKeyFileBalances(C)
						},
					},
					{
						Name:        "split",
						HelpName:    "split",
						Aliases:     []string{"s"},
						Description: "create a new key file, split all the funds from the root account to new keys",
						ArgsUsage:   "-a ${amount of addresses to create} -b ${amount in ethers to keep in root key}",
						Flags: []cli.Flag{
							&cli.Int64Flag{Name: "addresses", Aliases: []string{"a"}},
							&cli.Int64Flag{Name: "buffer", Aliases: []string{"b"}},
						},
						Action: func(cCtx *cli.Context) error {
							addresses := cCtx.Int64("addresses")
							rootKeyBuffer := cCtx.Int64("buffer")
							opts := &seth.FundKeyFileCmdOpts{Addrs: addresses, RootKeyBuffer: rootKeyBuffer}
							return seth.UpdateAndSplitFunds(C, opts)
						},
					},
					{
						Name:        "return",
						HelpName:    "return",
						Aliases:     []string{"r"},
						Description: "returns all the funds from addresses from keyfile.toml to original root key (KEYS env var)",
						ArgsUsage:   "-a ${addr_to_return_to}",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "address", Aliases: []string{"a"}},
						},
						Action: func(cCtx *cli.Context) error {
							toAddr := cCtx.String("address")
							return seth.ReturnFundsAndUpdateKeyfile(C, toAddr)
						},
					},
					{
						Name:        "remove",
						Aliases:     []string{"rm"},
						Description: "removes keyfile.toml",
						HelpName:    "return",
						Action: func(cCtx *cli.Context) error {
							return os.Remove(C.Cfg.KeyFilePath)
						},
					},
				},
			},
			{
				Name:        "trace",
				HelpName:    "trace",
				Aliases:     []string{"t"},
				Description: "trace transactions loaded from JSON file",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "file", Aliases: []string{"f"}},
				},
				Action: func(cCtx *cli.Context) error {
					file := cCtx.String("file")
					var transactions []string
					err := seth.OpenJsonFileAsStruct(file, &transactions)
					if err != nil {
						return err
					}

					_ = os.Setenv(seth.LogLevelEnvVar, "debug")

					cfgPath := os.Getenv("SETH_CONFIG_PATH")
					if cfgPath == "" {
						return errors.New(seth.ErrEmptyConfigPath)
					}
					var cfg *seth.Config
					d, err := os.ReadFile(cfgPath)
					if err != nil {
						return errors.Wrap(err, seth.ErrReadSethConfig)
					}
					err = toml.Unmarshal(d, &cfg)
					if err != nil {
						return errors.Wrap(err, seth.ErrUnmarshalSethConfig)
					}
					absPath, err := filepath.Abs(cfgPath)
					if err != nil {
						return err
					}
					cfg.ConfigDir = filepath.Dir(absPath)

					snet := os.Getenv("NETWORK")
					if snet == "" {
						return errors.New(ErrNoNetwork)
					}

					for _, n := range cfg.Networks {
						if n.Name == snet {
							cfg.Network = n
						}
					}
					if cfg.Network == nil {
						return fmt.Errorf("network %s not found", snet)
					}

					zero := int64(0)
					cfg.EphemeralAddrs = &zero

					client, err := seth.NewClientWithConfig(cfg)
					if err != nil {
						return err
					}

					seth.L.Info().Msgf("Tracing transactions from %s file", file)

					for _, tx := range transactions {
						seth.L.Info().Msgf("Tracing transaction %s", tx)
						err = client.Tracer.TraceGethTX(tx)
						if err != nil {
							return err
						}
					}
					return err
				},
			},
		},
	}
	return app.Run(args)
}
