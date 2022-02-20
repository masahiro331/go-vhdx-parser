package main

import (
	"github.com/masahiro331/go-disk"
	"github.com/masahiro331/go-vhdx-parser/pkg/vhdx"
	"io"
	"log"
	"os"
)

func main() {
	v, err := vhdx.Open("demo-1.0.20220210.0229.vhdx")
	if err != nil {
		log.Fatal(err)
	}

	driver, err := disk.NewDriver(v)
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("primary.ext4")
	if err != nil {
		log.Fatal(err)
	}
	for {
		p, err := driver.Next()
		if err != nil {
			log.Fatal(err)
		}
		if !p.Bootable() {
			for {
				_, err := io.CopyN(f, p, 512)
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	}
}
