package cloudconfig

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const cloudConfigHeader = "#cloud-config"

// topLevelFields is the allow-list of cloud-config keys this transpiler
// understands. Anything else must fail loudly rather than be dropped.
var topLevelFields = map[string]bool{
	"users":               true,
	"groups":              true,
	"ssh_authorized_keys": true,
	"write_files":         true,
	"runcmd":              true,
	"bootcmd":             true,
	"hostname":            true,
	"fqdn":                true,
}

// ParseError is a single problem found at a specific line of the source
// document. Parse returns these joined together (via errors.Join) so a
// document with several problems reports all of them at once.
type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

// Parse decodes cloud-config YAML into a Config. It strips an optional
// leading "#cloud-config" header line and rejects any top-level field
// outside the supported feature set with a named error instead of
// silently ignoring it.
func Parse(data []byte) (*Config, error) {
	data = stripHeader(data)

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, errors.Join(splitYAMLError(err)...)
	}
	if len(doc.Content) == 0 {
		return &Config{}, nil
	}

	root := doc.Content[0]

	var errs []error
	if root.Kind == yaml.MappingNode {
		errs = append(errs, unsupportedFields(root)...)
	}

	var cfg Config
	if err := root.Decode(&cfg); err != nil {
		errs = append(errs, splitYAMLError(err)...)
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &cfg, nil
}

func unsupportedFields(root *yaml.Node) []error {
	var errs []error
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		if !topLevelFields[key.Value] {
			errs = append(errs, &ParseError{Line: key.Line, Msg: fmt.Sprintf("unsupported field: %s", key.Value)})
		}
	}
	return errs
}

func stripHeader(data []byte) []byte {
	firstLine, rest, found := bytes.Cut(data, []byte("\n"))
	if !isHeaderLine(firstLine) {
		return data
	}
	if !found {
		return nil
	}
	// Blank the header line rather than deleting it, so every later
	// line keeps its original line number for error reporting.
	return append([]byte("\n"), rest...)
}

func isHeaderLine(line []byte) bool {
	return strings.HasPrefix(strings.TrimRight(string(line), "\r"), cloudConfigHeader)
}

// typeErr builds an error in the shape yaml.v3 expects back from a custom
// UnmarshalYAML: wrapping it in *yaml.TypeError lets decoding continue
// and collect other problems elsewhere in the document, instead of
// aborting on the first bad field.
func typeErr(node *yaml.Node, format string, args ...interface{}) error {
	return &yaml.TypeError{Errors: []string{fmt.Sprintf("line %d: %s", node.Line, fmt.Sprintf(format, args...))}}
}

// splitYAMLError turns yaml.v3's error output - either a single "yaml:
// line N: msg" syntax error or a multi-line *yaml.TypeError - into
// individual ParseErrors, one per problem.
func splitYAMLError(err error) []error {
	var terr *yaml.TypeError
	var lines []string
	if errors.As(err, &terr) {
		lines = terr.Errors
	} else {
		lines = []string{strings.TrimPrefix(err.Error(), "yaml: ")}
	}

	out := make([]error, 0, len(lines))
	for _, l := range lines {
		out = append(out, parseErrorFromLine(l))
	}
	return out
}

func parseErrorFromLine(s string) error {
	rest, ok := strings.CutPrefix(s, "line ")
	if !ok {
		return &ParseError{Msg: s}
	}
	n, msg, ok := strings.Cut(rest, ": ")
	if !ok {
		return &ParseError{Msg: s}
	}
	line, err := strconv.Atoi(n)
	if err != nil {
		return &ParseError{Msg: s}
	}
	return &ParseError{Line: line, Msg: msg}
}
