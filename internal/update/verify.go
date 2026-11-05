package update

import (
	"archive/zip"
	"bytes"
	"debug/pe"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TechSavage1989/weightmaxer/internal/version"
)

// Files a release zip may contain. Anything else rejects the download.
var allowedEntries = map[string]bool{
	ExeName:                   true,
	"README.txt":              true,
	"THIRD_PARTY_NOTICES.txt": true,
}

const (
	maxEntries   = 8
	maxExeSize   = 80 << 20
	maxOtherSize = 1 << 20
	maxRatio     = 50 // uncompressed/compressed; Go executables compress ~2-3x
)

// ExtractExe validates the release zip and writes Weightmaxer.exe to dir.
// It rejects path traversal, unexpected or duplicate files, links,
// decompression bombs and executables that are not Weightmaxer wantVersion.
func ExtractExe(zipPath, wantVersion, dir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("the download isn't a valid zip file: %w", err)
	}
	defer zr.Close()
	if len(zr.File) == 0 || len(zr.File) > maxEntries {
		return "", fmt.Errorf("the download has %d files; expected a few", len(zr.File))
	}
	seen := map[string]bool{}
	var exe *zip.File
	for _, f := range zr.File {
		if err := checkEntry(f); err != nil {
			return "", err
		}
		key := strings.ToLower(f.Name)
		if seen[key] {
			return "", fmt.Errorf("the download contains %s twice", f.Name)
		}
		seen[key] = true
		if f.Name == ExeName {
			exe = f
		}
	}
	if exe == nil {
		return "", errors.New("the download doesn't contain Weightmaxer.exe")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	out := filepath.Join(dir, ExeName)
	if err := extract(exe, out); err != nil {
		os.Remove(out)
		return "", err
	}
	if err := CheckExecutable(out, wantVersion); err != nil {
		os.Remove(out)
		return "", err
	}
	return out, nil
}

func checkEntry(f *zip.File) error {
	n := f.Name
	if n == "" || strings.ContainsAny(n, `\/:`) || n == "." || n == ".." || strings.HasPrefix(n, ".") {
		return fmt.Errorf("the download contains an unsafe path %q", n)
	}
	if !allowedEntries[n] {
		return fmt.Errorf("the download contains an unexpected file %q", n)
	}
	if !f.Mode().IsRegular() {
		return fmt.Errorf("%s in the download is not a regular file", n)
	}
	limit := uint64(maxOtherSize)
	if n == ExeName {
		limit = maxExeSize
	}
	if f.UncompressedSize64 == 0 || f.UncompressedSize64 > limit {
		return fmt.Errorf("%s in the download has an unexpected size", n)
	}
	if f.CompressedSize64 == 0 || f.UncompressedSize64/f.CompressedSize64 > maxRatio {
		return fmt.Errorf("%s in the download is compressed suspiciously well", n)
	}
	return nil
}

func extract(f *zip.File, out string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	w, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	// Read at most one byte more than declared so a lying header can't expand
	// past its stated size; archive/zip checks the CRC at the end.
	n, err := io.Copy(w, io.LimitReader(rc, int64(f.UncompressedSize64)+1))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("couldn't unpack %s: %w", f.Name, err)
	}
	if uint64(n) != f.UncompressedSize64 {
		return fmt.Errorf("%s in the download is damaged", f.Name)
	}
	return nil
}

// CheckExecutable confirms path is a 64-bit Windows GUI program carrying the
// Weightmaxer identity marker for exactly wantVersion.
func CheckExecutable(path, wantVersion string) error {
	f, err := pe.Open(path)
	if err != nil {
		return errors.New("the downloaded program isn't a Windows executable")
	}
	defer f.Close()
	if f.FileHeader.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return errors.New("the downloaded program isn't built for 64-bit Windows")
	}
	ch := f.FileHeader.Characteristics
	if ch&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 || ch&pe.IMAGE_FILE_DLL != 0 {
		return errors.New("the downloaded file isn't a program")
	}
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok || oh.Subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_GUI {
		return errors.New("the downloaded program isn't a Windows desktop app")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte(version.IdentityFor(wantVersion))) {
		return fmt.Errorf("the downloaded program isn't Weightmaxer %s", wantVersion)
	}
	return nil
}
