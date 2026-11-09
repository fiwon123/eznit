package main

import (
	"bytes"
	"encoding/binary"
)

// COFF machine and relocation constants. A .syso is a COFF object; Go's linker
// reads the .rsrc section from it and copies it into the PE as the resource
// directory, patching each OffsetToData through the relocation below.
const (
	machineI386  = 0x014c
	machineAMD64 = 0x8664
	machineARM64 = 0xaa64

	scnCntInitializedData = 0x00000040
	scnMemRead            = 0x40000000

	symClassStatic = 3

	relI386Dir32NB   = 0x0006
	relAMD64Addr32NB = 0x0003
	relARM64Addr32NB = 0x0002
)

const (
	coffHeaderSize    = 20
	sectionHeaderSize = 40
	symbolSize        = 18
	relocSize         = 10
)

// coff assembles the object: header, one section header, the section data, the
// relocations, and the section symbol they point at. The symbol table is what
// gives a relocation a target; without it the linker has nothing to resolve.
func coff(machine uint16, section []byte, relocs []reloc) []byte {
	var buffer bytes.Buffer

	dataOffset := align4Uint32(coffHeaderSize + sectionHeaderSize)

	symbols := []coffSymbol{{
		name:         ".rsrc",
		section:      1,
		storageClass: symClassStatic,
	}}

	symbolOffset := dataOffset + align4Uint32(uint32(len(section))) + uint32(relocSize*len(relocs))

	binary.Write(&buffer, binary.LittleEndian, struct {
		Machine              uint16
		NumberOfSections     uint16
		TimeDateStamp        uint32
		PointerToSymbolTable uint32
		NumberOfSymbols      uint32
		SizeOfOptionalHeader uint16
		Characteristics      uint16
	}{
		Machine:              machine,
		NumberOfSections:     1,
		PointerToSymbolTable: symbolOffset,
		NumberOfSymbols:      uint32(len(symbols)),
	})

	binary.Write(&buffer, binary.LittleEndian, struct {
		Name                 [8]byte
		VirtualSize          uint32
		VirtualAddress       uint32
		SizeOfRawData        uint32
		PointerToRawData     uint32
		PointerToRelocations uint32
		PointerToLinenumbers uint32
		NumberOfRelocations  uint16
		NumberOfLinenumbers  uint16
		Characteristics      uint32
	}{
		Name:                 sectionName(".rsrc"),
		VirtualSize:          uint32(len(section)),
		SizeOfRawData:        uint32(len(section)),
		PointerToRawData:     dataOffset,
		PointerToRelocations: dataOffset + align4Uint32(uint32(len(section))),
		NumberOfRelocations:  uint16(len(relocs)),
		Characteristics:      scnCntInitializedData | scnMemRead,
	})

	buffer.Write(make([]byte, int(dataOffset)-buffer.Len()))
	buffer.Write(section)

	for buffer.Len() < int(dataOffset+align4Uint32(uint32(len(section)))) {
		buffer.WriteByte(0)
	}

	for _, relocation := range relocs {
		binary.Write(&buffer, binary.LittleEndian, struct {
			VirtualAddress   uint32
			SymbolTableIndex uint32
			Type             uint16
		}{
			VirtualAddress: relocation.offset,
			Type:           relocationType(machine),
		})
	}

	for _, symbol := range symbols {
		binary.Write(&buffer, binary.LittleEndian, struct {
			Name               [8]byte
			Value              uint32
			SectionNumber      int16
			Type               uint16
			StorageClass       uint8
			NumberOfAuxSymbols uint8
		}{
			Name:          sectionName(symbol.name),
			SectionNumber: symbol.section,
			StorageClass:  symbol.storageClass,
		})
	}

	// An empty string table: COFF readers expect the length word after the
	// symbol table even when no symbol name needed one.
	binary.Write(&buffer, binary.LittleEndian, uint32(0))

	return buffer.Bytes()
}

type coffSymbol struct {
	name         string
	section      int16
	storageClass uint8
}

func relocationType(machine uint16) uint16 {
	switch machine {
	case machineI386:
		return relI386Dir32NB
	case machineARM64:
		return relARM64Addr32NB
	default:
		return relAMD64Addr32NB
	}
}

func sectionName(name string) [8]byte {
	var out [8]byte

	copy(out[:], name)

	return out
}

func align4Uint32(value uint32) uint32 {
	return (value + 3) &^ 3
}
