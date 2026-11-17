package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	arch := flag.String("arch", "amd64", "Windows target architecture")
	iconPath := flag.String("icon", "build/windows/icon.ico", "application icon")
	infoPath := flag.String("info", "build/windows/info.json", "Windows version metadata")
	outPath := flag.String("out", "wails_windows_amd64.syso", "output resource object")
	flag.Parse()

	if err := generate(*arch, *iconPath, *infoPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(arch, iconPath, infoPath, outPath string) error {
	architectures := map[string]winres.Arch{
		"amd64": winres.ArchAMD64,
		"arm64": winres.ArchARM64,
		"386":   winres.ArchI386,
	}
	resourceArch, ok := architectures[arch]
	if !ok {
		return fmt.Errorf("unsupported Windows resource architecture %q", arch)
	}

	iconFile, err := os.Open(iconPath)
	if err != nil {
		return err
	}
	icon, err := winres.LoadICO(iconFile)
	closeErr := iconFile.Close()
	if err != nil {
		return fmt.Errorf("load application icon: %w", err)
	}
	if closeErr != nil {
		return closeErr
	}

	infoData, err := os.ReadFile(infoPath)
	if err != nil {
		return err
	}
	var info version.Info
	if err := json.Unmarshal(infoData, &info); err != nil {
		return fmt.Errorf("read Windows version metadata: %w", err)
	}
	info.SetFileVersion("1.0.0.0")
	info.SetProductVersion("1.0.0.0")

	resources := &winres.ResourceSet{}
	if err := resources.SetIcon(winres.RT_ICON, icon); err != nil {
		return err
	}
	resources.SetVersionInfo(info)
	file, err := os.Create(outPath)
	if err != nil {
		return err
	}
	writeErr := resources.WriteObject(file, resourceArch)
	closeErr = file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}
