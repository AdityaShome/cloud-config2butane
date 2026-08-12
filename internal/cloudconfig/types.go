// Package cloudconfig models the subset of cloud-config YAML this transpiler
// understands: users, groups, ssh keys, write_files, runcmd/bootcmd and
// hostname/fqdn.
package cloudconfig

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level cloud-config document.
type Config struct {
	Users             []User      `yaml:"users"`
	Groups            GroupList   `yaml:"groups"`
	SSHAuthorizedKeys []string    `yaml:"ssh_authorized_keys"`
	WriteFiles        []File      `yaml:"write_files"`
	RunCmd            CommandList `yaml:"runcmd"`
	BootCmd           CommandList `yaml:"bootcmd"`
	Hostname          string      `yaml:"hostname"`
	FQDN              string      `yaml:"fqdn"`
}

// User is one entry of the top-level `users` list.
type User struct {
	Name              string    `yaml:"name"`
	Groups            CommaList `yaml:"groups"`
	Sudo              SudoRules `yaml:"sudo"`
	Shell             string    `yaml:"shell"`
	SSHAuthorizedKeys []string  `yaml:"ssh_authorized_keys"`
	Passwd            string    `yaml:"passwd"`
	LockPasswd        *bool     `yaml:"lock_passwd"`
	Gecos             string    `yaml:"gecos"`
	PrimaryGroup      string    `yaml:"primary_group"`
	NoCreateHome      bool      `yaml:"no_create_home"`
}

// Group is one entry of the top-level `groups` list: either a bare name or
// a name mapped to its initial members.
type Group struct {
	Name    string
	Members []string
}

// GroupList decodes `groups` in both its list-of-strings and
// list-of-maps forms, e.g.:
//
//	groups: [admin, docker]
//	groups:
//	  - admingroup: [root, sys]
//	  - docker
type GroupList []Group

func (l *GroupList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return typeErr(node, "groups: expected a list, got %s", kindName(node))
	}
	var out []Group
	for _, item := range node.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			out = append(out, Group{Name: item.Value})
		case yaml.MappingNode:
			// Walk key/value pairs directly instead of decoding into a
			// map, since map iteration order isn't stable and we want
			// groups to come out in document order.
			for i := 0; i+1 < len(item.Content); i += 2 {
				var members []string
				if err := item.Content[i+1].Decode(&members); err != nil {
					return typeErr(item.Content[i+1], "groups: invalid members for %q", item.Content[i].Value)
				}
				out = append(out, Group{Name: item.Content[i].Value, Members: members})
			}
		default:
			return typeErr(item, "groups: expected a name or a name-to-members map, got %s", kindName(item))
		}
	}
	*l = out
	return nil
}

// File is one entry of `write_files`.
type File struct {
	Path        string   `yaml:"path"`
	Content     string   `yaml:"content"`
	Owner       string   `yaml:"owner"`
	Permissions FileMode `yaml:"permissions"`
	Encoding    string   `yaml:"encoding"`
	Append      bool     `yaml:"append"`
}

// FileMode decodes `permissions`, written as either a quoted octal string
// ("0644") or a bare number. We parse the raw scalar text as base-8
// ourselves rather than trust YAML's own int resolution, since "644"
// without a leading zero is still meant as octal.
type FileMode struct {
	Value int
	isSet bool
}

func (m FileMode) IsSet() bool { return m.isSet }

func (m *FileMode) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return typeErr(node, "permissions: expected an octal string or number, got %s", kindName(node))
	}
	raw := strings.TrimSpace(node.Value)
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseInt(raw, 8, 32)
	if err != nil {
		return typeErr(node, "permissions: %q is not a valid octal mode", raw)
	}
	m.Value = int(v)
	m.isSet = true
	return nil
}

// Command is one entry of `runcmd`/`bootcmd`: either a shell line or an
// argv-style list.
type Command struct {
	Line string
	Argv []string
}

func (c Command) IsArgv() bool { return c.Argv != nil }

// CommandList decodes `runcmd`/`bootcmd`, where each entry is either a
// plain shell string or a list of argv words.
type CommandList []Command

func (l *CommandList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return typeErr(node, "expected a list of commands, got %s", kindName(node))
	}
	var out []Command
	for _, item := range node.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			out = append(out, Command{Line: item.Value})
		case yaml.SequenceNode:
			var argv []string
			if err := item.Decode(&argv); err != nil {
				return typeErr(item, "invalid argv-form command")
			}
			out = append(out, Command{Argv: argv})
		default:
			return typeErr(item, "expected a command string or an argv list, got %s", kindName(item))
		}
	}
	*l = out
	return nil
}

// CommaList decodes fields cloud-config documents as either a
// comma-separated string ("wheel, docker") or a YAML list, e.g. a user's
// `groups`.
type CommaList []string

func (l *CommaList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		s := strings.TrimSpace(node.Value)
		if s == "" {
			*l = nil
			return nil
		}
		parts := strings.Split(s, ",")
		out := make([]string, len(parts))
		for i, p := range parts {
			out[i] = strings.TrimSpace(p)
		}
		*l = out
		return nil
	case yaml.SequenceNode:
		var out []string
		if err := node.Decode(&out); err != nil {
			return typeErr(node, "expected a list of strings")
		}
		*l = out
		return nil
	default:
		return typeErr(node, "expected a string or a list of strings, got %s", kindName(node))
	}
}

// SudoRules decodes a user's `sudo` field: a single rule string, a list
// of rule strings, or the literal false meaning no sudo access at all
// (cloud-init's documented way to explicitly deny it). true has no
// defined meaning for this field and is rejected rather than guessed at.
type SudoRules []string

func (l *SudoRules) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!bool" {
			if node.Value == "false" {
				*l = nil
				return nil
			}
			return typeErr(node, "sudo: %q is not a supported value (use a rule string, a list of rules, or false)", node.Value)
		}
		if err := checkSudoRule(node); err != nil {
			return err
		}
		*l = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return typeErr(item, "sudo: expected a rule string")
			}
			if err := checkSudoRule(item); err != nil {
				return err
			}
			out = append(out, item.Value)
		}
		*l = out
		return nil
	default:
		return typeErr(node, "expected a string, a list of strings, or false, got %s", kindName(node))
	}
}

// checkSudoRule rejects an embedded newline in a sudo rule. Each rule
// becomes one line of a generated sudoers.d file (see convert.Users); a
// rule containing its own newline would inject an unrelated, independent
// sudoers line rather than staying part of this rule.
func checkSudoRule(node *yaml.Node) error {
	if strings.ContainsAny(node.Value, "\n\r") {
		return typeErr(node, "sudo: rule must not contain newlines")
	}
	return nil
}

func kindName(node *yaml.Node) string {
	switch node.Kind {
	case yaml.ScalarNode:
		return "a scalar"
	case yaml.SequenceNode:
		return "a list"
	case yaml.MappingNode:
		return "a map"
	default:
		return "an unsupported YAML node"
	}
}
