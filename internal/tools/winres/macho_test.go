package main

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"strings"
	"testing"
)

// machOHeader writes the 64-bit Mach-O header with no load commands: enough
// for debug/macho to read the magic, the CPU and the file type.
func machOHeader(cpu macho.Cpu, filetype macho.Type) []byte {
	var buffer bytes.Buffer

	buffer.Write([]byte{0xcf, 0xfa, 0xed, 0xfe}) // MH_MAGIC_64, little endian
	binary.Write(&buffer, binary.LittleEndian, uint32(cpu))
	binary.Write(&buffer, binary.LittleEndian, uint32(3)) // CPU_SUBTYPE_X86_64_ALL / ARM64_ALL
	binary.Write(&buffer, binary.LittleEndian, uint32(filetype))
	binary.Write(&buffer, binary.LittleEndian, uint32(0)) // ncmds
	binary.Write(&buffer, binary.LittleEndian, uint32(0)) // sizeofcmds
	binary.Write(&buffer, binary.LittleEndian, uint32(0)) // flags
	binary.Write(&buffer, binary.LittleEndian, uint32(0)) // reserved

	return buffer.Bytes()
}

func TestInspectMachO(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantCPU macho.Cpu
		wantErr bool
	}{
		{"amd64 executable", machOHeader(macho.CpuAmd64, macho.TypeExec), macho.CpuAmd64, false},
		{"arm64 executable", machOHeader(macho.CpuArm64, macho.TypeExec), macho.CpuArm64, false},
		{"an object, not an executable", machOHeader(macho.CpuAmd64, macho.TypeObj), macho.CpuAmd64, true},
		{"not Mach-O at all", []byte("\x7fELF not a mac binary"), 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, err := InspectMachO(tt.data)

			if (err != nil) != tt.wantErr {
				t.Fatalf("InspectMachO() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && cpu != tt.wantCPU {
				t.Errorf("CPU = %v, want %v", cpu, tt.wantCPU)
			}
		})
	}
}

func TestDarwinMachineForArchive(t *testing.T) {
	tests := []struct {
		name   string
		want   macho.Cpu
		wantOK bool
	}{
		{"mytool_v1.0.0_darwin_amd64.tar.gz", macho.CpuAmd64, true},
		{"mytool_v1.0.0_darwin_arm64.tar.gz", macho.CpuArm64, true},
		{"mytool_v1.0.0_linux_amd64.tar.gz", 0, false},
		{"mytool_v1.0.0_darwin.zip", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, ok := DarwinMachineForArchive(tt.name)

			if ok != tt.wantOK {
				t.Fatalf("DarwinMachineForArchive() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && cpu != tt.want {
				t.Errorf("CPU = %v, want %v", cpu, tt.want)
			}
		})
	}
}

// A darwin archive whose binary is the wrong CPU must fail: the platform is in
// the archive name, so an arm64 binary in the amd64 archive is a download that
// does not run.
func TestVerifyArchivesChecksDarwinMachO(t *testing.T) {
	dir := t.TempDir()
	folder := "mytool_v1.0.0_darwin_amd64"

	contents := map[string]string{
		folder + "/mytool":    string(machOHeader(macho.CpuArm64, macho.TypeExec)),
		folder + "/README.md": "r",
		folder + "/LICENSE":   "l",
	}
	writeTarGz(t, dir+"/"+folder+".tar.gz", contents)

	err := verifyArchives(dir, options())
	if err == nil {
		t.Fatal("an arm64 binary in the amd64 archive passed")
	}
	if !strings.Contains(err.Error(), "Mach-O CPU") {
		t.Fatalf("the wrong CPU was not named: %v", err)
	}
}

func TestVerifyArchivesAcceptsDarwinMachO(t *testing.T) {
	dir := t.TempDir()
	folder := "mytool_v1.0.0_darwin_arm64"

	contents := map[string]string{
		folder + "/mytool":    string(machOHeader(macho.CpuArm64, macho.TypeExec)),
		folder + "/README.md": "r",
		folder + "/LICENSE":   "l",
	}
	writeTarGz(t, dir+"/"+folder+".tar.gz", contents)

	if err := verifyArchives(dir, options()); err != nil {
		t.Fatalf("a good darwin archive failed: %v", err)
	}
}

func TestVerifyArchivesRejectsANonMachODarwinBinary(t *testing.T) {
	dir := t.TempDir()
	folder := "mytool_v1.0.0_darwin_amd64"

	contents := map[string]string{
		folder + "/mytool":    "\x7fELF a linux binary",
		folder + "/README.md": "r",
		folder + "/LICENSE":   "l",
	}
	writeTarGz(t, dir+"/"+folder+".tar.gz", contents)

	err := verifyArchives(dir, options())
	if err == nil || !strings.Contains(err.Error(), "Mach-O") {
		t.Fatalf("a non-Mach-O binary in a darwin archive was not rejected: %v", err)
	}
}
