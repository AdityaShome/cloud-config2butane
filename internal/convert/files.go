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

// Files converts cloud-config write_files entries into Butane storage
// files. Encoded content (base64/gzip/gz+b64) is decoded back to plain
// bytes so it can be embedded as inline content. append maps onto
// Butane's own File.Append, which Ignition supports natively.
func Files(files []cloudconfig.File) ([]butane.File, []error) {
	var out []butane.File
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

		out = append(out, bf)
	}

	return out, errs
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

func gunzip(data []byte) (string, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("invalid gzip content: %w", err)
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("invalid gzip content: %w", err)
	}
	return string(out), nil
}
