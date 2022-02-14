package main

import (
	"fmt"
	"github.com/masahiro331/go-vhdx-parser/pkg/vhdk"
	"github.com/masahiro331/go-vmdk-parser/pkg/disk"
	"io"
	"log"
	"os"
)

func main() {
	f, err := os.Open("demo-1.0.20220210.0229.vhdx")
	if err != nil {
		log.Fatal(err)
	}
	_, err = vhdk.NewVHDX(f)
	if err != nil {
		log.Fatal(err)
	}
	of, err := os.Open("output.file")
	if err != nil {
		log.Fatal(err)
	}

	driver, err := disk.NewDriver(of)
	if err != nil {
		log.Fatal(err)
	}

	for i, p := range driver.GetPartitions() {
		pf, err := os.Create(fmt.Sprintf("%s%d.img", p.Name(), i))
		if err != nil {
			log.Fatal(err)
		}

		_, err = of.Seek(int64(p.GetStartSector())*512, 0)
		if err != nil {
			log.Fatal(err)
		}

		reader := io.LimitReader(of, int64(p.GetSize())*512)
		i, err := io.Copy(pf, reader)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("file size:", i)
	}
}
