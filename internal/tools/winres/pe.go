// This file reads what a Windows executable says about itself: the machine it
// was built for, the subsystem that decides whether Windows opens a console,
// and the resources the shell draws its icon and Properties page from.
//
// It exists because a release can ship a file that looks right and is not: an
// arm64 build in the amd64 archive, or a GUI binary linked as a console program.
// Both are the difference between "double-click works" and "This app can't run
// on your PC", and neither is visible from the build command.
package main

import (
	"debug/pe"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Machine values from the COFF header.
const (
	MachineI386  = 0x014c
	MachineAMD64 = 0x8664
	MachineARM64 = 0xaa64
)

// Subsystem values from the PE optional header. A console program gets its own
// window from the loader; a GUI program does not.
const (
	SubsystemGUI     = 2
	SubsystemConsole = 3
)

// Resource types Windows defines, as stored in the resource directory.
const (
	RTIcon      = 3
	RTGroupIcon = 14
	RTVersion   = 16
	RTManifest  = 24
)

// Info is what the image declares.
type Info struct {
	Machine   uint16
	Subsystem uint16
	Resources map[uint32]bool
}

// Inspect reads the headers and the resource directory of an executable.
func Inspect(reader io.ReaderAt) (Info, error) {
	file, err := pe.NewFile(reader)
	if err != nil {
		return Info{}, fmt.Errorf("not a Windows executable: %w", err)
	}
	defer file.Close()

	info := Info{Machine: file.Machine, Subsystem: subsystemOf(file)}

	types, err := resourcePayloads(file)
	if err != nil {
		// No resource directory is a statement, not a failure: Inspect still
		// reports the machine and the subsystem.
		info.Resources = map[uint32]bool{}

		return info, nil
	}

	info.Resources = types

	return info, nil
}

// VerifyGUI reports what stops this image from being the window executable: it
// has to be a GUI program carrying the icon, manifest and version resources.
func (i Info) VerifyGUI() error {
	if i.Subsystem != SubsystemGUI {
		return fmt.Errorf("subsystem %d, want %d: a console binary opens a console window instead of the window", i.Subsystem, SubsystemGUI)
	}

	return i.VerifyResources()
}

// VerifyResources reports what stops this image from carrying the icon,
// manifest and version the shell draws from.
func (i Info) VerifyResources() error {
	for _, want := range []uint32{RTIcon, RTGroupIcon, RTManifest, RTVersion} {
		if !i.Resources[want] {
			return fmt.Errorf("no payload for resource type %d (icon 3, group icon 14, manifest 24, version 16); found %v", want, i.ResourceList())
		}
	}

	return nil
}

// VerifyConsole reports what stops this image from being a terminal program.
func (i Info) VerifyConsole() error {
	if i.Subsystem != SubsystemConsole {
		return fmt.Errorf("subsystem %d, want %d: a terminal command must keep its console", i.Subsystem, SubsystemConsole)
	}

	return nil
}

// VerifyMachine reports whether the image was built for the expected machine.
func (i Info) VerifyMachine(machine uint16) error {
	if i.Machine != machine {
		return fmt.Errorf("built for %s, want %s", MachineName(i.Machine), MachineName(machine))
	}

	return nil
}

// ResourceList is the resource types present, sorted, for an error message.
func (i Info) ResourceList() []uint32 {
	out := make([]uint32, 0, len(i.Resources))

	for kind := range i.Resources {
		out = append(out, kind)
	}

	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })

	return out
}

// MachineName names a COFF machine value the way a user reads it.
func MachineName(machine uint16) string {
	switch machine {
	case MachineI386:
		return "386 (32-bit x86)"
	case MachineAMD64:
		return "amd64 (64-bit Intel/AMD)"
	case MachineARM64:
		return "arm64 (Windows on ARM)"
	default:
		return fmt.Sprintf("machine %#x", machine)
	}
}

// MachineForArchive reads the architecture out of an archive name such as
// mytool_v0.20.16_windows_amd64.zip. The second result is false for an archive
// that is not a Windows one.
func MachineForArchive(name string) (uint16, bool) {
	base := strings.ToLower(name)

	switch {
	case strings.Contains(base, "_windows_amd64."):
		return MachineAMD64, true
	case strings.Contains(base, "_windows_arm64."):
		return MachineARM64, true
	case strings.Contains(base, "_windows_386."):
		return MachineI386, true
	}

	return 0, false
}

func subsystemOf(file *pe.File) uint16 {
	switch header := file.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		return header.Subsystem
	case *pe.OptionalHeader64:
		return header.Subsystem
	}

	return 0
}

// resourcePayloads lists the resource types that carry a payload in the image.
//
// The directory is a tree of three levels — type, name, language — and the
// payload hangs off the language level. A tree that stops at the name level
// still parses, and the loader refuses the image ("This app can't run on your
// PC"), so a type only counts here when its payload is reachable at the
// language level. That is what makes this check stricter than "a directory
// exists".
func resourcePayloads(file *pe.File) (map[uint32]bool, error) {
	var (
		rva  uint32
		size uint32
	)

	switch header := file.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		if len(header.DataDirectory) > 2 {
			rva, size = header.DataDirectory[2].VirtualAddress, header.DataDirectory[2].Size
		}
	case *pe.OptionalHeader64:
		if len(header.DataDirectory) > 2 {
			rva, size = header.DataDirectory[2].VirtualAddress, header.DataDirectory[2].Size
		}
	}

	if rva == 0 || size == 0 {
		return nil, fmt.Errorf("no resource directory")
	}

	data, err := sectionData(file, rva)
	if err != nil {
		return nil, err
	}

	offset := rva - sectionStart(file, rva)
	if int(offset)+16 > len(data) {
		return nil, fmt.Errorf("resource directory is truncated")
	}

	types := make(map[uint32]bool)

	var walk func(at int, path []uint32) error

	walk = func(at int, path []uint32) error {
		if at+16 > len(data) {
			return fmt.Errorf("resource directory at %#x is truncated", at)
		}

		count := int(le16(data[at+14:]))

		for index := 0; index < count; index++ {
			entry := at + 16 + 8*index
			if entry+8 > len(data) {
				return fmt.Errorf("resource entry at %#x is truncated", entry)
			}

			id := le32(data[entry:])
			target := le32(data[entry+4:])
			level := append(append([]uint32{}, path...), id)

			if target&0x80000000 != 0 {
				if len(level) > 3 {
					return fmt.Errorf("resource type %d nests deeper than three levels", path[0])
				}

				if err := walk(int(target&^0x80000000), level); err != nil {
					return err
				}

				continue
			}

			// A payload is only reachable the way the loader looks for it:
			// type, then name, then language.
			if len(level) == 3 {
				types[level[0]] = true
			}
		}

		return nil
	}

	if err := walk(int(offset), nil); err != nil {
		return nil, err
	}

	return types, nil
}

// sectionStart is the virtual address of the section holding rva.
func sectionStart(file *pe.File, rva uint32) uint32 {
	for _, section := range file.Sections {
		if rva >= section.VirtualAddress && rva < section.VirtualAddress+section.Size {
			return section.VirtualAddress
		}
	}

	return 0
}

// sectionData returns the raw bytes of the section holding rva.
func sectionData(file *pe.File, rva uint32) ([]byte, error) {
	for _, section := range file.Sections {
		if rva >= section.VirtualAddress && rva < section.VirtualAddress+section.Size {
			return section.Data()
		}
	}

	return nil, fmt.Errorf("address %#x is not in any section", rva)
}

func le16(data []byte) uint16 {
	if len(data) < 2 {
		return 0
	}

	return uint16(data[0]) | uint16(data[1])<<8
}

func le32(data []byte) uint32 {
	if len(data) < 4 {
		return 0
	}

	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
}
