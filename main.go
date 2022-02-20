package main

import (
	"bytes"
	"encoding/hex"
	"github.com/masahiro331/go-disk"
	"github.com/masahiro331/go-vhdx-parser/pkg/vhdx"
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
	for {
		p, err := driver.Next()
		if err != nil {
			log.Fatal(err)
		}
		if !p.Bootable() {
			buf := bytes.NewBuffer(nil)
			for i := 0; i < 16; i++ {
				b := make([]byte, 512)
				n, err := p.Read(b)
				if err != nil || n != 512 {
					log.Fatal("error:", n, err)
				}
				buf.Write(b)
			}
			hex.Dumper(os.Stdout).Write(buf.Bytes())
			return
		}
	}
}
