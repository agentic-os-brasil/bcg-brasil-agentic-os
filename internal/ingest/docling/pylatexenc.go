package docling

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
)

// This exact authenticated source distribution has no wheel. Copy only its
// reviewed package tree; never execute setup.py, a build backend or a resolver.
func addPylatexenc(entries map[string]entry, input Input, body []byte) error {
	if input.Version != "2.10" || input.Filename != "pylatexenc-2.10.tar.gz" || input.SHA256 != "3dd8fd84eb46dc30bee1e23eaab8d8fb5a7f507347b23e5f38ad9675c84f40d3" || digest(body) != input.SHA256 {
		return errors.New("source_distribution_unsupported")
	}
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(io.LimitReader(gz, 2<<20))
	aliases := map[string]bool{}
	count, files := 0, 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		count++
		if count > 64 || !relativeName(strings.TrimSuffix(h.Name, "/")) || !strings.HasPrefix(h.Name, "pylatexenc-2.10/") || h.Size < 0 || h.Size > 1<<20 {
			return errors.New("unsafe_source_member")
		}
		alias := strings.ToLower(strings.TrimSuffix(h.Name, "/"))
		if aliases[alias] {
			return errors.New("source_member_alias")
		}
		aliases[alias] = true
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return errors.New("unsafe_source_member")
		}
		b, err := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if err != nil || int64(len(b)) != h.Size {
			return errors.New("source_member_size")
		}
		var name string
		if strings.HasPrefix(h.Name, "pylatexenc-2.10/pylatexenc/") {
			name = "site-packages/" + strings.TrimPrefix(h.Name, "pylatexenc-2.10/")
			files++
		} else if h.Name == "pylatexenc-2.10/LICENSE.txt" || h.Name == "pylatexenc-2.10/LICENSE" || h.Name == "pylatexenc-2.10/PKG-INFO" {
			name = "source-inputs/" + h.Name
		}
		if name != "" {
			if _, ok := entries[name]; ok {
				return errors.New("source_collision")
			}
			entries[name] = entry{b, 0444}
			if err := boundEntries(entries); err != nil {
				return err
			}
		}
	}
	if files != 17 {
		return errors.New("source_package_incomplete")
	}
	entries["source-inputs/"+input.Filename] = entry{body, 0444}
	return boundEntries(entries)
}
