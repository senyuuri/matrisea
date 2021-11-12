package main

import (
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli"
	"sea.com/matrisea/vmm"
)

// A CLI dev tool for fast access of VMM's functions
func main() {
	app := cli.NewApp()

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "cmd",
		},
	}

	app.Action = func(c *cli.Context) error {
		if c.String("cmd") == "prunevm" {
			v, err := vmm.NewVMM(getenv("DATA_DIR", "/tmp/matrisea"))
			if err != nil {
				fmt.Println(err.Error())
				os.Exit(-1)
			}
			v.VMPrune()
			return nil
		}
		fmt.Println("Usage: --cmd prunevm")
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}
