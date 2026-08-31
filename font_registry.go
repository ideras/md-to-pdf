package md2pdf

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-text/typesetting/font"
	"github.com/ideras/md-to-pdf/internal/renderer"
	"github.com/pelletier/go-toml/v2"
)

// FontRole is one logical font family. Regular is required; the other variants
// are optional and fall back to Regular when absent.
type FontRole struct {
	Name       string
	Regular    []byte
	Bold       []byte
	Italic     []byte
	BoldItalic []byte
}

// FontRegistry holds validated custom font roles. Its contents are immutable to
// callers so a registry can safely be reused across conversions.
type FontRegistry struct {
	roles []FontRole
}

// NewFontRegistry validates and snapshots custom font roles.
func NewFontRegistry(roles ...FontRole) (*FontRegistry, error) {
	return newFontRegistry(roles, nil)
}

func newFontRegistry(roles []FontRole, paths []map[string]string) (*FontRegistry, error) {
	var errs []error
	seen := make(map[string]struct{}, len(roles))
	clean := make([]FontRole, 0, len(roles))
	for i, role := range roles {
		name := normalizeFontRoleName(role.Name)
		if name == "" {
			errs = append(errs, fmt.Errorf("font %d: name is empty", i))
			continue
		}
		if name == "default" || name == "mono" {
			errs = append(errs, fmt.Errorf("font %d (%q): reserved font role", i, role.Name))
			continue
		}
		if _, ok := seen[name]; ok {
			errs = append(errs, fmt.Errorf("font %d (%q): duplicate font role", i, role.Name))
			continue
		}
		seen[name] = struct{}{}
		role.Name = name
		var rolePaths map[string]string
		if i < len(paths) {
			rolePaths = paths[i]
		}
		if err := validateFontRole(i, role, rolePaths); err != nil {
			errs = append(errs, err)
			continue
		}
		clean = append(clean, cloneFontRole(role))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &FontRegistry{roles: clean}, nil
}

func validateFontRole(index int, role FontRole, paths map[string]string) error {
	fields := []struct {
		name     string
		contents []byte
		required bool
	}{
		{"regular", role.Regular, true},
		{"bold", role.Bold, false},
		{"italic", role.Italic, false},
		{"bold_italic", role.BoldItalic, false},
	}
	var errs []error
	for _, field := range fields {
		if len(field.contents) == 0 {
			if field.required {
				errs = append(errs, fmt.Errorf("font %d (%q): %s font is empty", index, role.Name, field.name))
			}
			continue
		}
		if _, err := font.ParseTTF(bytes.NewReader(field.contents)); err != nil {
			if path := paths[field.name]; path != "" {
				errs = append(errs, fmt.Errorf("font %d (%q) %s path %q: invalid TTF: %w", index, role.Name, field.name, path, err))
			} else {
				errs = append(errs, fmt.Errorf("font %d (%q) %s: invalid TTF: %w", index, role.Name, field.name, err))
			}
		}
	}
	return errors.Join(errs...)
}

func cloneFontRole(role FontRole) FontRole {
	role.Regular = bytes.Clone(role.Regular)
	role.Bold = bytes.Clone(role.Bold)
	role.Italic = bytes.Clone(role.Italic)
	role.BoldItalic = bytes.Clone(role.BoldItalic)
	return role
}

func normalizeFontRoleName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

type fontConfig struct {
	Fonts []fontConfigRole `toml:"fonts"`
}

type fontConfigRole struct {
	Name       string `toml:"name"`
	Regular    string `toml:"regular"`
	Bold       string `toml:"bold"`
	Italic     string `toml:"italic"`
	BoldItalic string `toml:"bold_italic"`
}

// LoadFontRegistry reads and fully validates a TOML font configuration. It
// never returns a partially valid registry.
func LoadFontRegistry(configPath string) (*FontRegistry, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read font config %q: %w", configPath, err)
	}
	var config fontConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse font config %q: %w", configPath, err)
	}

	roles := make([]FontRole, len(config.Fonts))
	paths := make([]map[string]string, len(config.Fonts))
	var errs []error
	for i, entry := range config.Fonts {
		roles[i].Name = entry.Name
		paths[i] = make(map[string]string)
		for _, field := range []struct {
			name string
			path string
			dst  *[]byte
		}{
			{"regular", entry.Regular, &roles[i].Regular},
			{"bold", entry.Bold, &roles[i].Bold},
			{"italic", entry.Italic, &roles[i].Italic},
			{"bold_italic", entry.BoldItalic, &roles[i].BoldItalic},
		} {
			if field.path == "" {
				continue
			}
			paths[i][field.name] = field.path
			contents, readErr := os.ReadFile(field.path)
			if readErr != nil {
				errs = append(errs, fmt.Errorf("font %d (%q) %s path %q: %w", i, entry.Name, field.name, field.path, readErr))
				continue
			}
			*field.dst = contents
		}
	}
	registry, validationErr := newFontRegistry(roles, paths)
	if validationErr != nil {
		errs = append(errs, validationErr)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return registry, nil
}

// WithFontRegistry supplies validated custom fonts for one conversion. A nil
// registry is equivalent to using only the embedded fonts.
func WithFontRegistry(registry *FontRegistry) Option {
	return func(o *options) { o.fonts = registry }
}

func (r *FontRegistry) rendererRoles() []renderer.FontRole {
	if r == nil {
		return nil
	}
	roles := make([]renderer.FontRole, len(r.roles))
	for i, role := range r.roles {
		roles[i] = renderer.FontRole{
			Name:       role.Name,
			Regular:    bytes.Clone(role.Regular),
			Bold:       bytes.Clone(role.Bold),
			Italic:     bytes.Clone(role.Italic),
			BoldItalic: bytes.Clone(role.BoldItalic),
		}
	}
	return roles
}
