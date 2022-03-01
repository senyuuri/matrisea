package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"time"

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
			dataDir := getenv("DATA_DIR", "/tmp/matrisea")
			cfPrefix := getenv("CF_PREFIX", "matrisea-test-")
			devicesDir := path.Join(dataDir, "devices")
			v := vmm.NewVMMImpl(dataDir, cfPrefix, 120*time.Second)
			v.VMPrune()
			if err := os.RemoveAll(devicesDir); err != nil {
				log.Fatalln(err.Error())
			}
			if err := os.Mkdir(devicesDir, 0755); err != nil {
				log.Fatalln(err.Error())
			}
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
