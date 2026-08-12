package convert

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	butane "github.com/coreos/butane/base/v0_5"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

// systemdUnitDir is where a write_files entry is recognized as a systemd
// unit rather than a generic file.
const systemdUnitDir = "/etc/systemd/system/"

// systemdUnitSuffixes are the unit types this transpiler passes through
// to systemd.units[] when found under systemdUnitDir.
var systemdUnitSuffixes = []string{".service", ".socket", ".timer", ".path", ".mount", ".target"}

// isSystemdUnitPath reports whether path names a unit file directly under
// systemdUnitDir - not a drop-in override (foo.service.d/override.conf),
// which is left as a generic file since it isn't a complete unit itself.
func isSystemdUnitPath(path string) bool {
	name, ok := strings.CutPrefix(path, systemdUnitDir)
	if !ok || strings.Contains(name, "/") {
		return false
	}
	for _, suffix := range systemdUnitSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// Files converts cloud-config write_files entries into Butane storage
// files. Encoded content (base64/gzip/gz+b64) is decoded back to plain
// bytes so it can be embedded as inline content. append maps onto
// Butane's own File.Append, which Ignition supports natively. Entries
// that look like systemd unit files (see isSystemdUnitPath) are routed
// to systemd.units[] instead, enabled by default, so their [Install]
// section actually takes effect.
func Files(files []cloudconfig.File) ([]butane.File, []butane.Unit, []error) {
	var outFiles []butane.File
	var outUnits []butane.Unit
	var errs []error

	for _, f := range files {
		if f.Path == "" {
			errs = append(errs, fmt.Errorf("write_files: missing path"))
			continue
		}

		content, err := decodeContent(f.Content, f.Encoding)
		if err != nil {
			errs = append(errs, fmt.Errorf("write_files[%s]: %w", f.Path, err))
			continue
		}

		if isSystemdUnitPath(f.Path) {
			name, _ := strings.CutPrefix(f.Path, systemdUnitDir)
			outUnits = append(outUnits, butane.Unit{
				Name:     name,
				Enabled:  boolPtr(true),
				Contents: strPtr(content),
			})
			continue
		}

		bf := butane.File{Path: f.Path}

		if f.Owner != "" {
			user, group, hasGroup := strings.Cut(f.Owner, ":")
			if user != "" {
				bf.User = butane.NodeUser{Name: strPtr(user)}
			}
			if hasGroup && group != "" {
				bf.Group = butane.NodeGroup{Name: strPtr(group)}
			}
		}

		// A private key found in the content wins over whatever
		// permissions the source specified - loose modes on key
		// material are a common cloud-config mistake we shouldn't
		// carry forward.
		if isPrivateKey(content) {
			bf.Mode = intPtr(0600)
		} else if f.Permissions.IsSet() {
			bf.Mode = intPtr(f.Permissions.Value)
		}

		resource := butane.Resource{Inline: &content}
		if f.Append {
			bf.Append = []butane.Resource{resource}
		} else {
			bf.Contents = resource
		}

		outFiles = append(outFiles, bf)
	}

	return outFiles, outUnits, errs
}

func isPrivateKey(content string) bool {
	return strings.Contains(content, "PRIVATE KEY")
}

func decodeContent(content, encoding string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "text/plain":
		return content, nil
	case "base64", "b64":
		raw, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return "", fmt.Errorf("invalid base64 content: %w", err)
		}
		return string(raw), nil
	case "gzip", "gz":
		return gunzip([]byte(content))
	case "gzip+base64", "gz+base64", "gzip+b64", "gz+b64":
		raw, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return "", fmt.Errorf("invalid base64 content: %w", err)
		}
		return gunzip(raw)
	default:
		return "", fmt.Errorf("unsupported encoding: %s", encoding)
	}
}

// maxDecompressedContentSize guards against a decompression bomb. A var,
// not a const, so tests can shrink it instead of decompressing tens of MB.
var maxDecompressedContentSize int64 = 64 << 20 // 64 MiB

func gunzip(data []byte) (string, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("invalid gzip content: %w", err)
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, maxDecompressedContentSize+1))
	if err != nil {
		return "", fmt.Errorf("invalid gzip content: %w", err)
	}
	if int64(len(out)) > maxDecompressedContentSize {
		return "", fmt.Errorf("gzip content decompresses to more than %d bytes", maxDecompressedContentSize)
	}
	return string(out), nil
}
