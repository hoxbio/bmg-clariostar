package main

import (
	"fmt"
	"log"
	"os"

	"github.com/hoxbio/bmg-clariostar/bmg"
)

var usage = `

Usage: bmg-clariostar verb

For now the only verb is the non-verb qubit, which runs a sbs 96w pcr plate
for the raw qubit fl values



`

func main() {

	args := os.Args

	if len(args) < 2 {
		fmt.Println("no verb provided")
		fmt.Println(usage)
		return
	}

	switch args[1] {
	case "qubit":
		c, err := bmg.Open("/dev/clario")
		if err != nil {
			log.Fatalf("could not open dev: %s", err)
		}

		// qubit config
		fl := bmg.FlCfg{
			Ex:           483,
			ExBw:         14,
			Dich:         5025,
			Em:           530,
			EmBw:         30,
			Gain:         3000,
			FocalHeight:  40,
			Flashes:      200,
			SettlingTime: 0,
		}
		// read all wells
		pl := bmg.PlateCfg{
			Length:      12776,
			Width:       8548,
			CornerX:     1438,
			CornerY:     1124,
			Cols:        12,
			Rows:        8,
			StartCorner: bmg.TopLeft,
		}
		rc := bmg.RunCfg{Plate: pl}
		c.RunFl(rc, fl)
		c.Close()
	}

}
