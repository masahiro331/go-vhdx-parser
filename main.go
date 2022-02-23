package main

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"

	disk "github.com/masahiro331/go-disk"
	"github.com/masahiro331/go-ext4-filesystem/ext4"
	"github.com/masahiro331/go-vhdx-parser/pkg/vhdx"
	"golang.org/x/xerrors"
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
		if p.Bootable() {
			continue
		}

		filesystem, err := ext4.NewFS(p.GetSectionReader())
		if err != nil {
			log.Fatal(err)
		}
		err = fs.WalkDir(filesystem, "/", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return xerrors.Errorf("file walk error: %w", err)
			}
			if d.IsDir() {
				return nil
			}

			fmt.Println(path)
			if path == "/usr/lib/os-release" {
				of, _ := os.Create("os-release")
				defer of.Close()

				sf, err := filesystem.Open(path)
				if err != nil {
					return err
				}
				io.Copy(of, sf)
			}
			return nil
		})
		if err != nil {
			log.Fatalf("%+v\n", err)
		}

		return
	}
}
