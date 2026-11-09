package main

import (
	"bytes"
	"debug/macho"
	"fmt"
	"strings"
)

// InspectMachO checks that data is a Mach-O executable and returns the CPU it
// was built for. It is the macOS counterpart of Inspect: a tar.gz whose binary
// is a Linux ELF, or an arm64 build in the amd64 archive, looks fine until the
// user runs it.
func InspectMachO(data []byte) (macho.Cpu, error) {
	file, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("not a Mach-O binary: %w", err)
	}
	defer file.Close()

	if file.Type != macho.TypeExec {
		return file.Cpu, fmt.Errorf("Mach-O type is %v, want an executable", file.Type)
	}

	return file.Cpu, nil
}

// DarwinMachineForArchive maps an archive name to the CPU it must carry. The
// platform is in the name, as it is for every archive; a darwin archive whose
// name does not say amd64 or arm64 cannot be checked and is refused by the
// caller.
func DarwinMachineForArchive(name string) (macho.Cpu, bool) {
	base := strings.ToLower(name)

	switch {
	case strings.Contains(base, "_darwin_amd64."):
		return macho.CpuAmd64, true
	case strings.Contains(base, "_darwin_arm64."):
		return macho.CpuArm64, true
	}

	return 0, false
}
