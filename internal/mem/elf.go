package mem

import (
	"bytes"
	"debug/elf"
	"fmt"
	"io"
)

type LoadedImage struct {
	Entry uint64
	IsELF bool
}

func LoadELFOrRaw(b *Bus, base uint64, data []byte) (LoadedImage, error) {
	if len(data) >= 4 && data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
		ef, err := elf.NewFile(bytes.NewReader(data))
		if err != nil {
			return LoadedImage{}, err
		}
		defer ef.Close()
		if ef.Class != elf.ELFCLASS64 || ef.Data != elf.ELFDATA2LSB {
			return LoadedImage{}, fmt.Errorf("only 64-bit little-endian RISC-V ELF is supported")
		}
		for _, p := range ef.Progs {
			if p.Type != elf.PT_LOAD {
				continue
			}
			if p.Memsz == 0 {
				continue
			}
			if p.Filesz > 0 {
				buf := make([]byte, p.Filesz)
				r := p.Open()
				if _, err := io.ReadFull(r, buf); err != nil {
					return LoadedImage{}, err
				}
				if err := b.Load(p.Paddr, buf); err != nil {
					return LoadedImage{}, err
				}
			}
			if p.Memsz > p.Filesz {
				z := make([]byte, p.Memsz-p.Filesz)
				if err := b.Load(p.Paddr+p.Filesz, z); err != nil {
					return LoadedImage{}, err
				}
			}
		}
		return LoadedImage{Entry: ef.Entry, IsELF: true}, nil
	}
	if err := b.Load(base, data); err != nil {
		return LoadedImage{}, err
	}
	return LoadedImage{Entry: base, IsELF: false}, nil
}
