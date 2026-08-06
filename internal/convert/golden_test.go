package convert

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/AdityaShome/cloud-config2butane/internal/butaneout"
	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

// TestGoldenFixtures runs every testdata/golden/<case>/input.yaml through
// the full pipeline and compares it against expected.bu. Comparison is
// semantic (unmarshaled YAML, not raw bytes) since key order in a
// mapping isn't meaningful.
func TestGoldenFixtures(t *testing.T) {
	dirs, err := filepath.Glob("../../testdata/golden/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no golden fixtures found")
	}

	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join(dir, "input.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			wantRaw, err := os.ReadFile(filepath.Join(dir, "expected.bu"))
			if err != nil {
				t.Fatal(err)
			}

			cfg, err := cloudconfig.Parse(input)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			butaneCfg, errs := Convert(cfg)
			if len(errs) != 0 {
				t.Fatalf("Convert: %v", errs)
			}
			gotRaw, err := butaneout.Marshal(butaneCfg)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var got, want interface{}
			if err := yaml.Unmarshal(gotRaw, &got); err != nil {
				t.Fatalf("unmarshal actual output: %v", err)
			}
			if err := yaml.Unmarshal(wantRaw, &want); err != nil {
				t.Fatalf("unmarshal expected.bu: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("mismatch\n--- got ---\n%s\n--- want ---\n%s", gotRaw, wantRaw)
			}
		})
	}
}
