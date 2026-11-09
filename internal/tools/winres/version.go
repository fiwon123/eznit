package main

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
	"unicode/utf16"
)

// versionInfo builds RT_VERSION's payload: the fixed file info Explorer reads
// for Properties → Details, plus the strings that give the binary a description
// and a product name instead of a bare file name.
//
// The structure is a tree of length-prefixed blocks; see VS_VERSIONINFO. Every
// string is UTF-16 with a trailing NUL, and each child starts on a four-byte
// boundary.
func versionInfo(name, description, company, version string) []byte {
	numbers := numericVersion(version)

	fixed := fixedFileInfo(numbers)

	strings := vsNode("StringFileInfo", nil, 0, 1,
		vsNode("040904B0", nil, 0, 1,
			vsString("CompanyName", company),
			vsString("FileDescription", description),
			vsString("FileVersion", displayVersion(version)),
			vsString("InternalName", name),
			vsString("OriginalFilename", name+".exe"),
			vsString("ProductName", name),
			vsString("ProductVersion", displayVersion(version)),
		),
	)

	translation := vsNode("VarFileInfo", nil, 0, 1,
		vsNode("Translation", []byte{0x09, 0x04, 0xB0, 0x04}, 4, 0),
	)

	return vsNode("VS_VERSION_INFO", fixed, uint16(len(fixed)), 0, strings, translation)
}

// vsNode assembles one block: its length, the value length and type, the
// UTF-16 key, then its value and children on four-byte boundaries. The length
// written back includes the children but not padding after the block itself.
func vsNode(key string, value []byte, valueLength uint16, kind uint16, children ...[]byte) []byte {
	var buffer bytes.Buffer

	binary.Write(&buffer, binary.LittleEndian, uint16(0)) // length, patched below
	binary.Write(&buffer, binary.LittleEndian, valueLength)
	binary.Write(&buffer, binary.LittleEndian, kind)

	writeUTF16(&buffer, key)
	pad4(&buffer)
	buffer.Write(value)

	for _, child := range children {
		pad4(&buffer)
		buffer.Write(child)
	}

	out := buffer.Bytes()
	binary.LittleEndian.PutUint16(out[0:2], uint16(len(out)))

	return out
}

func vsString(key, value string) []byte {
	encoded := utf16.Encode([]rune(value + "\x00"))

	return vsNode(key, utf16Bytes(encoded), uint16(len(encoded)), 1)
}

func writeUTF16(buffer *bytes.Buffer, value string) {
	buffer.Write(utf16Bytes(utf16.Encode([]rune(value + "\x00"))))
}

func utf16Bytes(encoded []uint16) []byte {
	out := make([]byte, 2*len(encoded))

	for index, unit := range encoded {
		binary.LittleEndian.PutUint16(out[2*index:], unit)
	}

	return out
}

func pad4(buffer *bytes.Buffer) {
	for buffer.Len()%4 != 0 {
		buffer.WriteByte(0)
	}
}

// fixedFileInfo is the 52-byte VS_FIXEDFILEINFO block.
func fixedFileInfo(version [4]uint16) []byte {
	fileMS := uint32(version[0])<<16 | uint32(version[1])
	fileLS := uint32(version[2])<<16 | uint32(version[3])

	values := []uint32{
		0xFEEF04BD, // signature
		0x00010000, // structure version
		fileMS,     // file version MS
		fileLS,     // file version LS
		fileMS,     // product version MS
		fileLS,     // product version LS
		0x0000003F, // file flags mask
		0x00000000, // file flags
		0x00040004, // VOS_NT_WINDOWS32
		0x00000001, // VFT_APP
		0x00000000, // subtype
		0x00000000, // file date MS
		0x00000000, // file date LS
	}

	out := make([]byte, 0, 4*len(values))

	for _, value := range values {
		out = binary.LittleEndian.AppendUint32(out, value)
	}

	return out
}

// numericVersion reads the first four numbers out of a release tag, so v0.20.15
// and 0.20.15-rc1 both become 0.20.15.0.
func numericVersion(version string) [4]uint16 {
	var out [4]uint16

	fields := strings.FieldsFunc(version, func(r rune) bool {
		return r < '0' || r > '9'
	})

	for index, field := range fields {
		if index >= len(out) {
			break
		}

		value, err := strconv.ParseUint(field, 10, 16)
		if err != nil {
			break
		}

		out[index] = uint16(value)
	}

	return out
}

// displayVersion is the string Explorer shows beside the numbers: the tag as
// released, or a stable placeholder when the build had no tag.
func displayVersion(version string) string {
	if strings.TrimSpace(version) == "" {
		return "v0.0.0"
	}

	return version
}
