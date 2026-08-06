package convert

import (
	"strings"

	butane "github.com/coreos/butane/base/v0_5"
)

const hostnamePath = "/etc/hostname"

// Hostname converts `hostname`/`fqdn` into a storage.files entry writing
// /etc/hostname. An explicit hostname wins if both are set; if only fqdn
// is given, the short hostname is its first label. Returns nil if
// neither field is set.
func Hostname(hostname, fqdn string) *butane.File {
	name := hostname
	if name == "" {
		name, _, _ = strings.Cut(fqdn, ".")
	}
	if name == "" {
		return nil
	}

	content := name + "\n"
	return &butane.File{
		Path:     hostnamePath,
		Mode:     intPtr(0644),
		Contents: butane.Resource{Inline: &content},
	}
}
