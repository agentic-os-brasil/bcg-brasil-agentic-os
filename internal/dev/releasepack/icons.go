package releasepack

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// NativeIconOutputs records the deterministic platform icon assets generated
// from the canonical Maestro SVG. The native signatures are applied later by
// the platform release jobs; this command never signs or claims trust.
type NativeIconOutputs struct {
	Source       string              `json:"source"`
	SourceSHA256 string              `json:"source_sha256"`
	ICNS         string              `json:"icns"`
	ICNSSHA256   string              `json:"icns_sha256"`
	ICO          string              `json:"ico"`
	ICOSHA256    string              `json:"ico_sha256"`
	Manifest     string              `json:"manifest"`
	Rasterizer   string              `json:"rasterizer"`
	Toolchain    NativeIconToolchain `json:"toolchain"`
}

// NativeIconToolchain fingerprints the exact system binaries that rasterize
// and package the source. The release manifest can therefore distinguish
// same-tool-name output produced by different macOS images.
type NativeIconToolchain struct {
	QLManage NativeIconTool `json:"qlmanage"`
	Sips     NativeIconTool `json:"sips"`
	Iconutil NativeIconTool `json:"iconutil"`
}

type NativeIconTool struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type iconSize struct {
	name string
	size int
}

var maestroIconSet = []iconSize{
	{name: "icon_16x16.png", size: 16},
	{name: "icon_16x16@2x.png", size: 32},
	{name: "icon_32x32.png", size: 32},
	{name: "icon_32x32@2x.png", size: 64},
	{name: "icon_128x128.png", size: 128},
	{name: "icon_128x128@2x.png", size: 256},
	{name: "icon_256x256.png", size: 256},
	{name: "icon_256x256@2x.png", size: 512},
	{name: "icon_512x512.png", size: 512},
	{name: "icon_512x512@2x.png", size: 1024},
}

// BuildNativeIcons rasterizes the canonical SVG with the macOS system
// renderer, emits the native macOS .icns and a PNG-backed Windows .ico, then
// records all output hashes. A release workflow must run this before native
// signing; no platform signature is produced here.
func BuildNativeIcons(ctx context.Context, source, outputDir string) (NativeIconOutputs, error) {
	if runtime.GOOS != "darwin" {
		return NativeIconOutputs{}, fmt.Errorf("native icon factory requires macOS qlmanage/sips/iconutil; run it on a macOS release worker")
	}
	toolchain, err := collectIconToolchain()
	if err != nil {
		return NativeIconOutputs{}, err
	}
	if err := requireRegularFile(source, "icon source"); err != nil {
		return NativeIconOutputs{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return NativeIconOutputs{}, fmt.Errorf("create icon output directory: %w", err)
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return NativeIconOutputs{}, fmt.Errorf("normalize icon source: %w", err)
	}
	sourceSHA256, err := fileSHA256(source)
	if err != nil {
		return NativeIconOutputs{}, fmt.Errorf("hash icon source: %w", err)
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return NativeIconOutputs{}, fmt.Errorf("normalize icon output directory: %w", err)
	}
	tempDir, err := os.MkdirTemp("", "maestro-native-icons-")
	if err != nil {
		return NativeIconOutputs{}, fmt.Errorf("create icon work directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	renderDir := filepath.Join(tempDir, "render")
	iconsetDir := filepath.Join(tempDir, "maestro.iconset")
	if err := os.MkdirAll(renderDir, 0o755); err != nil {
		return NativeIconOutputs{}, err
	}
	if err := runIconCommand(ctx, "qlmanage", "-t", "-s", "1024", "-o", renderDir, source); err != nil {
		return NativeIconOutputs{}, err
	}
	rendered := filepath.Join(renderDir, filepath.Base(source)+".png")
	if err := requireRegularFile(rendered, "rasterized icon"); err != nil {
		return NativeIconOutputs{}, err
	}
	if err := os.MkdirAll(iconsetDir, 0o755); err != nil {
		return NativeIconOutputs{}, err
	}
	for _, icon := range maestroIconSet {
		if err := runIconCommand(ctx, "sips", "-z", fmt.Sprint(icon.size), fmt.Sprint(icon.size), rendered, "--out", filepath.Join(iconsetDir, icon.name)); err != nil {
			return NativeIconOutputs{}, err
		}
	}
	icnsPath := filepath.Join(outputDir, "maestro-app-icon.icns")
	if err := runIconCommand(ctx, "iconutil", "-c", "icns", iconsetDir, "-o", icnsPath); err != nil {
		return NativeIconOutputs{}, err
	}
	pngs := make([][]byte, 0, 5)
	for _, name := range []string{"icon_16x16.png", "icon_32x32.png", "icon_32x32@2x.png", "icon_128x128.png", "icon_256x256.png"} {
		data, readErr := os.ReadFile(filepath.Join(iconsetDir, name))
		if readErr != nil {
			return NativeIconOutputs{}, fmt.Errorf("read generated icon %s: %w", name, readErr)
		}
		pngs = append(pngs, data)
	}
	icoPath := filepath.Join(outputDir, "maestro-app-icon.ico")
	if err := os.WriteFile(icoPath, buildICO(pngs), 0o644); err != nil {
		return NativeIconOutputs{}, fmt.Errorf("write Windows icon: %w", err)
	}
	result := NativeIconOutputs{
		Source:       source,
		SourceSHA256: sourceSHA256,
		ICNS:         icnsPath,
		ICO:          icoPath,
		Manifest:     filepath.Join(outputDir, "maestro-app-icon-manifest.json"),
		Rasterizer:   "macOS qlmanage + sips + iconutil",
		Toolchain:    toolchain,
	}
	currentSourceSHA256, err := fileSHA256(source)
	if err != nil {
		return NativeIconOutputs{}, fmt.Errorf("re-hash icon source: %w", err)
	}
	if currentSourceSHA256 != sourceSHA256 {
		return NativeIconOutputs{}, fmt.Errorf("icon source changed while native assets were generated")
	}
	result.ICNSSHA256, err = fileSHA256(icnsPath)
	if err != nil {
		return NativeIconOutputs{}, fmt.Errorf("hash generated icns: %w", err)
	}
	result.ICOSHA256, err = fileSHA256(icoPath)
	if err != nil {
		return NativeIconOutputs{}, fmt.Errorf("hash generated ico: %w", err)
	}
	manifestResult := result
	manifestResult.Source = filepath.Base(source)
	manifestResult.ICNS = filepath.Base(icnsPath)
	manifestResult.ICO = filepath.Base(icoPath)
	manifestResult.Manifest = filepath.Base(result.Manifest)
	manifest, err := json.MarshalIndent(manifestResult, "", "  ")
	if err != nil {
		return NativeIconOutputs{}, fmt.Errorf("encode icon manifest: %w", err)
	}
	if err := os.WriteFile(result.Manifest, append(manifest, '\n'), 0o644); err != nil {
		return NativeIconOutputs{}, fmt.Errorf("write icon manifest: %w", err)
	}
	return result, nil
}

func buildICO(pngs [][]byte) []byte {
	const headerSize = 6
	const entrySize = 16
	result := make([]byte, headerSize+entrySize*len(pngs))
	// ICONDIR: reserved=0, type=1 (icon), count=N.
	binary.LittleEndian.PutUint16(result[2:4], 1)
	binary.LittleEndian.PutUint16(result[4:6], uint16(len(pngs)))
	offset := uint32(len(result))
	for index, png := range pngs {
		entry := result[headerSize+index*entrySize : headerSize+(index+1)*entrySize]
		size := iconSizeForIndex(index)
		if size >= 256 {
			entry[0], entry[1] = 0, 0
		} else {
			entry[0], entry[1] = byte(size), byte(size)
		}
		binary.LittleEndian.PutUint16(entry[4:6], 1)
		binary.LittleEndian.PutUint16(entry[6:8], 32)
		binary.LittleEndian.PutUint32(entry[8:12], uint32(len(png)))
		binary.LittleEndian.PutUint32(entry[12:16], offset)
		result = append(result, png...)
		offset += uint32(len(png))
	}
	return result
}

func iconSizeForIndex(index int) int {
	return []int{16, 32, 64, 128, 256}[index]
}

func runIconCommand(ctx context.Context, command string, args ...string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s failed: %w (%s)", command, err, string(output))
	}
	return nil
}

func collectIconToolchain() (NativeIconToolchain, error) {
	qlmanage, err := fingerprintIconTool("qlmanage")
	if err != nil {
		return NativeIconToolchain{}, err
	}
	sips, err := fingerprintIconTool("sips")
	if err != nil {
		return NativeIconToolchain{}, err
	}
	iconutil, err := fingerprintIconTool("iconutil")
	if err != nil {
		return NativeIconToolchain{}, err
	}
	return NativeIconToolchain{QLManage: qlmanage, Sips: sips, Iconutil: iconutil}, nil
}

func fingerprintIconTool(command string) (NativeIconTool, error) {
	path, err := exec.LookPath(command)
	if err != nil {
		return NativeIconTool{}, fmt.Errorf("native icon factory requires %s: %w", command, err)
	}
	digest, err := fileSHA256(path)
	if err != nil {
		return NativeIconTool{}, fmt.Errorf("fingerprint %s: %w", command, err)
	}
	return NativeIconTool{Path: path, SHA256: digest}, nil
}

func requireRegularFile(path, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s is unavailable: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file: %s", label, path)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
