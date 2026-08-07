// Command cloud-config2butane converts cloud-config YAML into Butane YAML
// for Flatcar.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/AdityaShome/cloud-config2butane/internal/butaneout"
	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
	"github.com/AdityaShome/cloud-config2butane/internal/convert"
	"github.com/AdityaShome/cloud-config2butane/internal/validate"
)

func main() {
	in := flag.String("in", "", "path to the cloud-config YAML input file (required)")
	out := flag.String("out", "", "path to write the generated Butane YAML (default: stdout)")
	doValidate := flag.Bool("validate", true, "validate the generated Butane config against the real coreos/butane library")
	jsonErrors := flag.Bool("json", false, "on failure, print errors as JSON instead of plain text")
	flag.Parse()

	if *in == "" {
		fmt.Fprintln(os.Stderr, "cloud-config2butane: -in is required")
		os.Exit(2)
	}

	if err := run(*in, *out, *doValidate); err != nil {
		if *jsonErrors {
			printJSONError(err)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func printJSONError(err error) {
	out, marshalErr := json.MarshalIndent(struct {
		Success bool     `json:"success"`
		Errors  []string `json:"errors"`
	}{Errors: flattenErrors(err)}, "", "  ")
	if marshalErr != nil {
		// json marshaling of a []string can't actually fail; this is
		// just a safety net in case that ever changes.
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprintln(os.Stderr, string(out))
}

// flattenErrors walks both errors.Join's Unwrap() []error and
// fmt.Errorf's Unwrap() error down to the individual leaf errors, so
// -json reports each distinct problem as its own array entry instead of
// one giant string.
func flattenErrors(err error) []string {
	if err == nil {
		return nil
	}
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		var out []string
		for _, e := range u.Unwrap() {
			out = append(out, flattenErrors(e)...)
		}
		return out
	}
	if u, ok := err.(interface{ Unwrap() error }); ok {
		if inner := u.Unwrap(); inner != nil {
			return flattenErrors(inner)
		}
	}
	return []string{err.Error()}
}

func run(in, out string, doValidate bool) error {
	data, err := os.ReadFile(in)
	if err != nil {
		return fmt.Errorf("reading %s: %w", in, err)
	}

	cfg, err := cloudconfig.Parse(data)
	if err != nil {
		return fmt.Errorf("parsing %s:\n%w", in, err)
	}

	butaneCfg, errs := convert.Convert(cfg)
	if len(errs) > 0 {
		return fmt.Errorf("converting %s:\n%w", in, errors.Join(errs...))
	}

	yamlOut, err := butaneout.Marshal(butaneCfg)
	if err != nil {
		return fmt.Errorf("marshaling generated butane config: %w", err)
	}

	if doValidate {
		if err := validate.Ignition(yamlOut); err != nil {
			return fmt.Errorf("validating generated butane config: %w", err)
		}
	}

	if out == "" {
		_, err = os.Stdout.Write(yamlOut)
		return err
	}
	return os.WriteFile(out, yamlOut, 0644)
}
