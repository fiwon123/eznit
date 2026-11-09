package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// testPNG draws a small opaque image, so the generator has something to scale.
func testPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))

	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 4), B: 40, A: 255})
		}
	}

	var buffer bytes.Buffer

	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	return buffer.Bytes()
}

// The generator produces one decodable PNG per size Windows asks for.
func TestIconImagesCoverEverySize(t *testing.T) {
	images, err := iconImages(bytes.NewReader(testPNG(t)))
	if err != nil {
		t.Fatalf("iconImages: %v", err)
	}

	if len(images) != len(iconSizes) {
		t.Fatalf("images = %d, want %d", len(images), len(iconSizes))
	}

	for index, image := range images {
		decoded, err := png.Decode(bytes.NewReader(image.png))
		if err != nil {
			t.Fatalf("%d px icon does not decode: %v", image.size, err)
		}

		if got := decoded.Bounds().Dx(); got != iconSizes[index] {
			t.Errorf("icon %d is %d px, want %d", index, got, iconSizes[index])
		}
	}
}

// The group icon names every individual icon resource; a mismatch is a blank
// icon in Explorer rather than an error.
func TestGroupIconReferencesTheIconResources(t *testing.T) {
	images, err := iconImages(bytes.NewReader(testPNG(t)))
	if err != nil {
		t.Fatalf("iconImages: %v", err)
	}

	group := groupIcon(images)

	if count := binary.LittleEndian.Uint16(group[4:]); int(count) != len(images) {
		t.Fatalf("group icon count = %d, want %d", count, len(images))
	}

	for index := range images {
		at := 6 + 14*index
		id := binary.LittleEndian.Uint16(group[at+12:])

		if int(id) != index+1 {
			t.Errorf("entry %d points at resource %d, want %d", index, id, index+1)
		}
	}
}

// resourceEntry is one leaf found by walking the generated tree.
type resourceEntry struct {
	path []uint32
	data []byte
}

// walkResources parses the .rsrc section the way the loader does: the root's
// entries are resource types, then names, then languages, and only then a data
// entry. It returns every leaf keyed by type.
func walkResources(t *testing.T, section []byte) map[uint32][]resourceEntry {
	t.Helper()

	found := make(map[uint32][]resourceEntry)

	var walk func(at uint32, path []uint32)

	walk = func(at uint32, path []uint32) {
		count := binary.LittleEndian.Uint16(section[at+14:])

		for index := uint16(0); index < count; index++ {
			entryAt := at + 16 + uint32(8*index)
			id := binary.LittleEndian.Uint32(section[entryAt:])
			offset := binary.LittleEndian.Uint32(section[entryAt+4:])
			level := append(append([]uint32{}, path...), id)

			// Every offset in the tree is relative to the start of the section,
			// not to the directory that holds it.
			if offset&0x80000000 != 0 {
				walk(offset&^0x80000000, level)
				continue
			}

			entry := offset
			address := binary.LittleEndian.Uint32(section[entry:])
			size := binary.LittleEndian.Uint32(section[entry+4:])

			if len(level) != 3 {
				t.Fatalf("resource %v is at %d levels, want type → name → language", level, len(level))
			}

			found[level[0]] = append(found[level[0]], resourceEntry{
				path: level,
				data: section[address : address+size],
			})
		}
	}

	walk(0, nil)

	return found
}

// The tree carries the four resource types the icon, manifest and version need,
// with the payloads intact and every leaf under a language directory.
func TestResourcesCarryIconManifestAndVersion(t *testing.T) {
	images, err := iconImages(bytes.NewReader(testPNG(t)))
	if err != nil {
		t.Fatalf("iconImages: %v", err)
	}

	manifestXML := manifest("com.example.mytool-gui", "a sample tool", "v0.20.15")
	version := versionInfo("mytool-gui", "a sample tool", "example", "v0.20.15")

	section, relocs, err := resources(images, manifestXML, version)
	if err != nil {
		t.Fatalf("resources: %v", err)
	}

	found := walkResources(t, section)

	if len(found[rtIcon]) != len(images) {
		t.Errorf("icon resources = %d, want %d", len(found[rtIcon]), len(images))
	}

	for index, entry := range found[rtIcon] {
		if int(entry.path[1]) != index+1 {
			t.Errorf("icon %d has name id %d, want %d", index, entry.path[1], index+1)
		}

		if entry.path[2] != neutralLanguage {
			t.Errorf("icon %d is stored under language %d, want neutral", entry.path[1], entry.path[2])
		}

		if !bytes.HasPrefix(entry.data, []byte("\x89PNG")) {
			t.Errorf("icon %d is not PNG data", entry.path[1])
		}
	}

	if len(found[rtGroupIcon]) != 1 || len(found[rtGroupIcon][0].data) == 0 {
		t.Error("the group icon resource is missing")
	}

	if len(found[rtManifest]) != 1 || !bytes.Contains(found[rtManifest][0].data, []byte("dpiAware")) {
		t.Error("the manifest resource is missing or not DPI aware")
	}

	for _, entry := range found[rtManifest] {
		if entry.path[2] != neutralLanguage {
			t.Errorf("the manifest is stored under language %d, want neutral", entry.path[2])
		}
	}

	if len(found[rtVersion]) != 1 || !bytes.Equal(found[rtVersion][0].data, version) {
		t.Error("the version resource is missing or differs")
	}

	// Every data entry needs a relocation, and every relocation has to point at
	// a four-byte field inside the section: an unrelocated entry makes Windows
	// read its payload at file offset zero.
	if want := len(found[rtIcon]) + len(found[rtGroupIcon]) + len(found[rtManifest]) + len(found[rtVersion]); len(relocs) != want {
		t.Errorf("relocations = %d, want one per data entry (%d)", len(relocs), want)
	}

	for _, relocation := range relocs {
		if int(relocation.offset)+4 > len(section) {
			t.Errorf("relocation at %d is outside the section", relocation.offset)
		}
	}
}

// The version block is a tree of length-prefixed nodes; every length has to
// agree with the bytes that follow or Explorer shows an empty Properties page.
func TestVersionInfoIsSelfConsistent(t *testing.T) {
	version := versionInfo("mytool", "a sample tool", "example", "v0.20.15")

	values := make(map[string]string)

	var walk func(base uint32)

	walk = func(base uint32) {
		t.Helper()

		length := binary.LittleEndian.Uint16(version[base:])
		valueLength := binary.LittleEndian.Uint16(version[base+2:])
		kind := binary.LittleEndian.Uint16(version[base+4:])

		if int(base)+int(length) > len(version) {
			t.Fatalf("node at %d claims %d bytes past the block", base, length)
		}

		keyEnd := base + 6

		for binary.LittleEndian.Uint16(version[keyEnd:]) != 0 {
			keyEnd += 2
		}

		key := utf16String(version[base+6 : keyEnd])

		valueAt := (keyEnd + 2 + 3) &^ 3
		valueBytes := uint32(valueLength)
		if kind == 1 {
			valueBytes *= 2
		}

		if kind == 1 && valueLength > 0 {
			values[key] = utf16String(version[valueAt : valueAt+valueBytes])
		}

		if key == "VS_VERSION_INFO" {
			signature := binary.LittleEndian.Uint32(version[valueAt:])
			if signature != 0xFEEF04BD {
				t.Errorf("fixed file info signature = %#x, want 0xFEEF04BD", signature)
			}
		}

		child := (valueAt + valueBytes + 3) &^ 3
		end := base + uint32(length)

		for child+6 <= end {
			childLength := binary.LittleEndian.Uint16(version[child:])
			if childLength == 0 {
				break
			}

			walk(child)
			child = (child + uint32(childLength) + 3) &^ 3
		}
	}

	walk(0)

	if values["ProductName"] != "mytool" {
		t.Errorf("ProductName = %q, want mytool", values["ProductName"])
	}

	if !strings.Contains(values["FileDescription"], "sample tool") {
		t.Errorf("FileDescription = %q, want the description", values["FileDescription"])
	}

	if values["FileVersion"] != "v0.20.15" {
		t.Errorf("FileVersion = %q, want the tag", values["FileVersion"])
	}
}

// utf16String decodes a UTF-16LE key.
func utf16String(data []byte) string {
	units := make([]uint16, 0, len(data)/2)
	for index := 0; index+1 < len(data); index += 2 {
		units = append(units, binary.LittleEndian.Uint16(data[index:]))
	}

	runes := make([]rune, 0, len(units))
	for _, unit := range units {
		if unit == 0 {
			break
		}

		runes = append(runes, rune(unit))
	}

	return string(runes)
}

// The manifest names the app and asks for per-monitor DPI.
func TestManifestIsDPIAndLongPathAware(t *testing.T) {
	body := string(manifest("com.example.mytool-gui", "a sample tool", "v0.20.15"))

	for _, want := range []string{"com.example.mytool-gui", "permonitorv2", "longPathAware", `level="asInvoker"`, `version="0.20.15.0"`} {
		if !strings.Contains(body, want) {
			t.Errorf("manifest does not contain %q", want)
		}
	}
}

func TestNumericVersion(t *testing.T) {
	tests := []struct {
		version string
		want    [4]uint16
	}{
		{"v0.20.15", [4]uint16{0, 20, 15, 0}},
		{"0.20.15-rc1", [4]uint16{0, 20, 15, 1}},
		{"v1.2", [4]uint16{1, 2, 0, 0}},
		{"", [4]uint16{}},
		{"vx.x.x", [4]uint16{}},
	}

	for _, tt := range tests {
		if got := numericVersion(tt.version); got != tt.want {
			t.Errorf("numericVersion(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

// The object is valid COFF for the linker: one .rsrc section, a section symbol
// and one relocation per data entry.
func TestCOFFObjectIsLinkable(t *testing.T) {
	images, err := iconImages(bytes.NewReader(testPNG(t)))
	if err != nil {
		t.Fatalf("iconImages: %v", err)
	}

	section, relocs, err := resources(images, manifest("com.example.mytool", "a sample tool", "v1.0.0"), versionInfo("mytool", "a sample tool", "example", "v1.0.0"))
	if err != nil {
		t.Fatalf("resources: %v", err)
	}

	object := coff(machineAMD64, section, relocs)

	file, err := pe.NewFile(bytes.NewReader(object))
	if err != nil {
		t.Fatalf("the linker would reject this object: %v", err)
	}
	defer file.Close()

	if file.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		t.Errorf("machine = %#x, want amd64", file.Machine)
	}

	if len(file.Sections) != 1 || file.Sections[0].Name != ".rsrc" {
		t.Fatalf("sections = %v, want one .rsrc", file.Sections)
	}

	if got := file.Sections[0].Characteristics; got != pe.IMAGE_SCN_CNT_INITIALIZED_DATA|pe.IMAGE_SCN_MEM_READ {
		t.Errorf("characteristics = %#x, want read-only initialised data", got)
	}

	if len(file.Symbols) != 1 || file.Symbols[0].Name != ".rsrc" {
		t.Errorf("symbols = %v, want the .rsrc section symbol", file.Symbols)
	}

	if int(file.Sections[0].NumberOfRelocations) != len(relocs) {
		t.Errorf("relocations = %d, want %d", file.Sections[0].NumberOfRelocations, len(relocs))
	}
}
