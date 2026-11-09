package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

// Resource types Windows defines. The names are the numeric ids stored in the
// resource directory, not strings: an id is enough for Explorer to find them.
const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
	rtManifest  = 24
)

// resource is one node of the resource directory tree: a directory when it has
// children, a leaf with payload bytes otherwise.
type resource struct {
	id       uint32
	value    []byte
	children []*resource
}

// neutralLanguage is the language id every resource is stored under. Zero is
// LANG_NEUTRAL, which a lookup for any language falls back to.
const neutralLanguage = 0

// leaf places a payload on the third level of the tree: type → name →
// language → data entry.
//
// The loader's activation-context lookup walks that path. A payload hung
// directly off the name directory still parses as a resource tree, but Windows
// refuses to start the image: "This app can't run on your PC" (#459).
func leaf(id uint32, value []byte) *resource {
	return &resource{id: id, children: []*resource{{id: neutralLanguage, value: value}}}
}

// reloc is a 4-byte field that must hold the address of the resource data
// relative to the image base, not relative to the section. Go's linker patches
// it from the relocation record.
type reloc struct {
	offset uint32
}

// resources lays out the whole resource tree and returns the bytes of the
// .rsrc section plus the fields that need relocating.
//
// The layout follows the format: every directory is a header and its entries,
// then every leaf gets a data entry, then the payloads. Offsets in the tree are
// relative to the section start; the four-byte OffsetToData of each data entry
// is the only field that needs a relocation.
func resources(icons []icon, manifestXML, version []byte) ([]byte, []reloc, error) {
	iconEntries := make([]*resource, 0, len(icons))

	for index, image := range sortedIcons(icons) {
		iconEntries = append(iconEntries, leaf(uint32(index+1), image.png))
	}

	root := &resource{children: []*resource{
		{id: rtIcon, children: iconEntries},
		{id: rtGroupIcon, children: []*resource{leaf(1, groupIcon(icons))}},
		{id: rtManifest, children: []*resource{leaf(1, manifestXML)}},
	}}

	if len(version) > 0 {
		root.children = append(root.children, &resource{
			id:       rtVersion,
			children: []*resource{leaf(1, version)},
		})
	}

	sort.Slice(root.children, func(i, j int) bool { return root.children[i].id < root.children[j].id })

	return layout(root)
}

// directory is one resource directory with its children in stored order.
type directory struct {
	node     *resource
	offset   uint32
	children []*resource
}

func layout(root *resource) ([]byte, []reloc, error) {
	directories := []*directory{}
	queue := []*resource{root}

	// Breadth first, so the root is always at offset zero.
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		children := append([]*resource(nil), node.children...)
		sort.Slice(children, func(i, j int) bool { return children[i].id < children[j].id })

		entry := &directory{node: node, children: children}
		directories = append(directories, entry)

		for _, child := range children {
			if len(child.children) > 0 {
				queue = append(queue, child)
			}
		}
	}

	var offset uint32

	for _, entry := range directories {
		entry.offset = offset
		offset += directorySize(len(entry.children))
	}

	dataEntries := make(map[*resource]uint32)

	for _, entry := range directories {
		for _, child := range entry.children {
			if len(child.children) > 0 {
				continue
			}

			dataEntries[child] = offset
			offset += dataEntrySize
		}
	}

	payload := make(map[*resource]uint32)

	for _, entry := range directories {
		for _, child := range entry.children {
			if len(child.children) > 0 {
				continue
			}

			offset = align4(offset)
			payload[child] = offset
			offset += uint32(len(child.value))
		}
	}

	section := make([]byte, offset)
	relocs := make([]reloc, 0, len(dataEntries))

	write := func(at uint32, value any) error {
		var buffer bytes.Buffer

		if err := binary.Write(&buffer, binary.LittleEndian, value); err != nil {
			return err
		}

		copy(section[at:], buffer.Bytes())

		return nil
	}

	for _, entry := range directories {
		if err := write(entry.offset, struct {
			Characteristics   uint32
			TimeDateStamp     uint32
			MajorVersion      uint16
			MinorVersion      uint16
			NumberOfNamed     uint16
			NumberOfIdEntries uint16
		}{NumberOfIdEntries: uint16(len(entry.children))}); err != nil {
			return nil, nil, err
		}

		at := entry.offset + directoryHeaderSize

		for _, child := range entry.children {
			address := dataEntries[child]

			if len(child.children) > 0 {
				address = 0x80000000 | offsetOf(directories, child)
			}

			if err := write(at, struct {
				Name         uint32
				OffsetToData uint32
			}{Name: child.id, OffsetToData: address}); err != nil {
				return nil, nil, err
			}

			at += 8
		}
	}

	for _, entry := range directories {
		for _, child := range entry.children {
			if len(child.children) > 0 {
				continue
			}

			at := dataEntries[child]
			relocs = append(relocs, reloc{offset: at})

			// The field holds the section-relative offset as the relocation's
			// addend; the linker adds the section's virtual address to it.
			if err := write(at, struct {
				OffsetToData uint32
				Size         uint32
				CodePage     uint32
				Reserved     uint32
			}{OffsetToData: payload[child], Size: uint32(len(child.value))}); err != nil {
				return nil, nil, err
			}

			copy(section[payload[child]:], child.value)
		}
	}

	return section, relocs, nil
}

const (
	directoryHeaderSize = 16
	dataEntrySize       = 16
)

func directorySize(children int) uint32 {
	return uint32(directoryHeaderSize + 8*children)
}

func offsetOf(directories []*directory, node *resource) uint32 {
	for _, entry := range directories {
		if entry.node == node {
			return entry.offset
		}
	}

	return 0
}

func align4(value uint32) uint32 {
	return (value + 3) &^ 3
}

// manifest is the application manifest: it opts into the common controls and
// per-monitor DPI, which is what keeps the window sharp instead of letting
// Windows scale a 96 DPI bitmap.
func manifest(appID, description, version string) []byte {
	return fmt.Appendf(nil, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="%s" version="%s" processorArchitecture="*"/>
  <description>%s</description>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">permonitorv2,permonitor</dpiAwareness>
      <longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware>
    </windowsSettings>
  </application>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
</assembly>
`, appID, manifestVersion(version), description)
}

// manifestVersion is the four-part version the manifest requires; it falls back
// to 1.0.0.0 when the build did not pass a tag.
func manifestVersion(version string) string {
	parts := numericVersion(version)
	if parts == [4]uint16{} {
		return "1.0.0.0"
	}

	return fmt.Sprintf("%d.%d.%d.%d", parts[0], parts[1], parts[2], parts[3])
}
