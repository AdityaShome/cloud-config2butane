// Package butaneout serializes an assembled Butane config to YAML.
package butaneout

import (
	"bytes"
	"strconv"

	"gopkg.in/yaml.v3"

	butane1_1 "github.com/coreos/butane/config/flatcar/v1_1"
)

// Marshal serializes cfg to YAML. Butane's own schema types carry no
// yaml `omitempty` tags, so a direct yaml.Marshal would litter the
// output with `field: null` and `field: []` for every unset optional
// field. We encode into a yaml.Node instead, prune those out, and
// reformat file modes as octal, so the result reads like a hand-written
// Butane file.
func Marshal(cfg *butane1_1.Config) ([]byte, error) {
	var node yaml.Node
	if err := node.Encode(cfg); err != nil {
		return nil, err
	}
	prune(&node)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func prune(node *yaml.Node) *yaml.Node {
	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for i, c := range node.Content {
			node.Content[i] = prune(c)
		}
	case yaml.MappingNode:
		var kept []*yaml.Node
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			val := prune(node.Content[i+1])
			if isEmpty(val) {
				continue
			}
			// mode is a permission bitmask; showing it in octal
			// ("0644") reads the way a human would write it,
			// unlike Go's plain decimal encoding of the same int.
			if key.Value == "mode" && val.Kind == yaml.ScalarNode {
				if n, err := strconv.Atoi(val.Value); err == nil {
					val.Value = "0" + strconv.FormatInt(int64(n), 8)
				}
			}
			kept = append(kept, key, val)
		}
		node.Content = kept
	}
	return node
}

func isEmpty(node *yaml.Node) bool {
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Tag == "!!null"
	case yaml.SequenceNode, yaml.MappingNode:
		return len(node.Content) == 0
	default:
		return false
	}
}
