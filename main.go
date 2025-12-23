package main

import (
	"os"

	"github.com/urfave/cli/v2"
	"github.com/w-h-a/gomento/cmd"
)

func main() {
	app := &cli.App{
		Name: "golens",
		Commands: []*cli.Command{
			{
				Name: "server",
				Action: func(ctx *cli.Context) error {
					return cmd.Run(ctx)
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "api_key",
						Usage:    "API key for LLM Model",
						Required: false,
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		os.Exit(1)
	}
}
