package seth

import (
	"errors"
	"os"

	"github.com/smartcontractkit/seth"
	"github.com/urfave/cli/v2"
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
			var err error
			C, err = seth.NewClient()
			if err != nil {
				return err
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
					_, err := ge.Stats(blocks, tipPerc)
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
						ArgsUsage:   "-a ${amount of addresses to create}",
						Flags: []cli.Flag{
							&cli.Int64Flag{Name: "addresses", Aliases: []string{"a"}},
						},
						Action: func(cCtx *cli.Context) error {
							addresses := cCtx.Int64("addresses")
							opts := &seth.FundKeyFileCmdOpts{Addrs: addresses}
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
							return seth.ReturnFunds(C, toAddr)
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
		},
	}
	return app.Run(args)
}
