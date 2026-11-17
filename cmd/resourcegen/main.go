package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	arch := flag.String("arch", "amd64", "Windows target architecture")
	iconPath := flag.String("icon", "build/windows/icon.ico", "application icon")
	infoPath := flag.String("info", "build/windows/info.json", "Windows version metadata")
	versionPath := flag.String("version", "version.go", "Go source that defines appVersion")
	outPath := flag.String("out", "wails_windows_amd64.syso", "output resource object")
	flag.Parse()

	if err := generate(*arch, *iconPath, *infoPath, *versionPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var appVersionPattern = regexp.MustCompile(`(?m)^\s*var appVersion\s*=\s*"(\d+\.\d+\.\d+)"`)

// appVersion reads the version that version.go gives the app, so the Windows
// file properties show the same version the in-app updater reports.
func appVersion(path string) (string, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	match := appVersionPattern.FindSubmatch(source)
	if match == nil {
		return "", fmt.Errorf("%s: no se encontró appVersion con formato x.y.z", path)
	}
	return string(match[1]), nil
}

func generate(arch, iconPath, infoPath, versionPath, outPath string) error {
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
	version, err := appVersion(versionPath)
	if err != nil {
		return err
	}
	info.SetFileVersion(version + ".0")
	info.SetProductVersion(version + ".0")

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
