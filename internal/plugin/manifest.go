package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	proto "github.com/Tejas242/ember/pkg/plugin"
)

// Manifest is a plugin's plugin.toml.
type Manifest struct {
	Name        string
	Description string
	Command     string
	Args        []string
	Icon        string
	Caps        []string            // the capabilities it declared that Ember knows
	Asks        []string            // the grants it asks for (only the user's config grants them)
	Targets     map[string][]string // a click's targets: a URL (one element) or a command
	Dir         string
	Mod         time.Time // the manifest's mtime: a change wakes a suspended plugin
}

// Has says whether the manifest declared capability c.
func (m *Manifest) Has(c string) bool { return slices.Contains(m.Caps, c) }

// IsURL says whether a target is a URL to open, not a command.
func IsURL(t []string) bool { return len(t) == 1 && strings.Contains(t[0], "://") }

// LoadManifest reads dir/plugin.toml.
func LoadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "plugin.toml")
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := parseManifest(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if m.Name != filepath.Base(dir) {
		return nil, fmt.Errorf("%s: name %q isn't its directory's, %q", path, m.Name, filepath.Base(dir))
	}
	m.Dir, m.Mod = dir, fi.ModTime()
	return m, nil
}

// parseManifest reads the small TOML a manifest uses: key = "string",
// key = ["array", "of", "strings"], # comments, and a [targets] table.
func parseManifest(text string) (*Manifest, error) {
	m := &Manifest{Targets: map[string][]string{}}
	section := ""
	for i, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(stripComment(raw))
		if ln == "" {
			continue
		}
		bad := func(f string, a ...any) error { return fmt.Errorf("line %d: %s", i+1, fmt.Sprintf(f, a...)) }
		if strings.HasPrefix(ln, "[") {
			if section = strings.TrimSpace(strings.Trim(ln, "[]")); section != "targets" {
				return nil, bad("unknown table [%s]", section)
			}
			continue
		}
		k, v, ok := strings.Cut(ln, "=")
		if !ok {
			return nil, bad("not key = value")
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		vals, isList, err := tomlValue(v)
		if err != nil {
			return nil, bad("%s: %v", k, err)
		}
		if section == "targets" {
			if !proto.NameRe.MatchString(k) || len(vals) == 0 {
				return nil, bad("target %q: a name and a URL or a command", k)
			}
			if !isList && !strings.Contains(vals[0], "://") {
				return nil, bad("target %q: a string must be a URL; a command is an array", k)
			}
			m.Targets[k] = vals
			continue
		}
		one := func() (string, error) {
			if isList {
				return "", fmt.Errorf("%s: a string, not an array", k)
			}
			return vals[0], nil
		}
		switch k {
		case "name":
			m.Name, err = one()
		case "description":
			m.Description, err = one()
		case "command":
			m.Command, err = one()
		case "icon":
			m.Icon, err = one()
		case "args":
			m.Args = vals
		case "grants":
			for _, g := range vals {
				if slices.Contains([]string{proto.GrantCritical, proto.GrantBadge, proto.GrantFocus}, g) && !slices.Contains(m.Asks, g) {
					m.Asks = append(m.Asks, g)
				}
			}
		case "capabilities":
			for _, c := range vals {
				if slices.Contains(proto.Caps, c) && !slices.Contains(m.Caps, c) {
					m.Caps = append(m.Caps, c)
				}
			}
		default: // a newer Ember's key: fine
		}
		if err != nil {
			return nil, bad("%v", err)
		}
	}
	switch {
	case !proto.NameRe.MatchString(m.Name):
		return nil, errors.New("name must be 1–32 of a-z, 0-9 and -")
	case m.Command == "":
		return nil, errors.New("no command")
	}
	m.Icon = proto.Cut(m.Icon, 32)
	return m, nil
}

// tomlValue reads a quoted string or an array of them.
func tomlValue(v string) (vals []string, list bool, err error) {
	if strings.HasPrefix(v, "[") {
		if !strings.HasSuffix(v, "]") {
			return nil, true, errors.New("an array must close on its line")
		}
		inner := strings.TrimSpace(v[1 : len(v)-1])
		for inner != "" {
			s, rest, err := tomlString(inner)
			if err != nil {
				return nil, true, err
			}
			vals = append(vals, s)
			inner = strings.TrimSpace(rest)
			if inner != "" {
				if inner[0] != ',' {
					return nil, true, errors.New("a comma between the strings")
				}
				inner = strings.TrimSpace(inner[1:])
			}
		}
		return vals, true, nil
	}
	s, rest, err := tomlString(v)
	if err == nil && strings.TrimSpace(rest) != "" {
		err = errors.New("text after the string")
	}
	return []string{s}, false, err
}

func tomlString(v string) (s, rest string, err error) {
	if v == "" || (v[0] != '"' && v[0] != '\'') {
		return "", "", errors.New("a quoted string")
	}
	q := v[0]
	if q == '\'' { // literal: no escapes
		end := strings.IndexByte(v[1:], '\'')
		if end < 0 {
			return "", "", errors.New("unclosed string")
		}
		return v[1 : 1+end], v[2+end:], nil
	}
	for i := 1; i < len(v); i++ {
		switch v[i] {
		case '\\':
			i++
		case '"':
			s, err := strconv.Unquote(v[:i+1])
			return s, v[i+1:], err
		}
	}
	return "", "", errors.New("unclosed string")
}

// stripComment cuts a # comment that isn't inside quotes.
func stripComment(ln string) string {
	quote := byte(0)
	for i := 0; i < len(ln); i++ {
		c := ln[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote == 0 && c == '#':
			return ln[:i]
		}
	}
	return ln
}
