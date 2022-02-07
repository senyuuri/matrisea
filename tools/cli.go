package main

import (
	"fmt"
	"log"
	"os"
	"path"

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
			devicesDir := path.Join(dataDir, "devices")
			v, err := vmm.NewVMM(dataDir)
			if err != nil {
				log.Fatalln(err.Error())
			}
			v.VMPrune()
			err = os.RemoveAll(devicesDir)
			if err != nil {
				log.Fatalln(err.Error())
			}
			os.Mkdir(devicesDir, 0755)
			if err != nil {
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
