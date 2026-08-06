// Package validate compiles generated Butane YAML through the real
// coreos/butane library, the same one the butane CLI uses. This is the
// correctness backstop: if real Butane rejects our output, that's a bug
// in the converter, not something to warn about and move past.
package validate

import (
	"fmt"

	"github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
)

// Ignition compiles butaneYAML to Ignition JSON, returning an error if
// Butane rejects it for any reason (schema violation, out-of-range
// value, etc.).
func Ignition(butaneYAML []byte) error {
	_, report, err := config.TranslateBytes(butaneYAML, common.TranslateBytesOptions{})
	if err != nil {
		return fmt.Errorf("butane rejected the generated config: %w", err)
	}
	if report.IsFatal() {
		return fmt.Errorf("butane rejected the generated config: %s", report.String())
	}
	return nil
}
