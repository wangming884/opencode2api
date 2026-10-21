package config

import (
	"fmt"
	"sort"
	"strings"
)

// aliasDepth bounds a chain of aliases. Loading rejects a loop outright, so
// this only protects a request served from a configuration that reached the
// gateway some other way.
const aliasDepth = 8

// ResolveModel returns the model that actually serves a request for model. A
// name that is not an alias resolves to itself, so callers apply it
// unconditionally.
func (c Config) ResolveModel(model string) string {
	return resolveAlias(c.Models.Aliases, model)
}

// Aliased reports whether model is a public name that routes to another model.
func (c Config) Aliased(model string) bool {
	return c.ResolveModel(model) != model
}

// Hides reports whether model must stay out of /v1/models.
func (m ModelsConfig) Hides(model string) bool {
	for _, hidden := range m.Hidden {
		if hidden == model {
			return true
		}
	}
	return false
}

func resolveAlias(aliases map[string]string, model string) string {
	if len(aliases) == 0 {
		return model
	}
	seen := map[string]bool{model: true}
	current := model
	for range aliasDepth {
		target, ok := aliases[current]
		if !ok || target == "" || seen[target] {
			return current
		}
		seen[target] = true
		current = target
	}
	return current
}

// normalizeModelAliases trims the alias table and rejects the shapes that would
// make routing ambiguous or non-terminating: an empty name, a name that maps to
// itself, a chain that loops, and a name that shadows a rotation alias (the
// alias table is consulted first, so the rotation group would be unreachable).
func normalizeModelAliases(cfg *Config) error {
	if len(cfg.Models.Aliases) == 0 {
		cfg.Models.Aliases = nil
	} else {
		aliases := make(map[string]string, len(cfg.Models.Aliases))
		for rawName, rawTarget := range cfg.Models.Aliases {
			name := strings.TrimSpace(rawName)
			target := strings.TrimSpace(rawTarget)
			if name == "" || target == "" {
				return fmt.Errorf("models.aliases must map a non-empty name to a non-empty model, got %q: %q", rawName, rawTarget)
			}
			if name == target {
				return fmt.Errorf("models.aliases maps %q to itself", name)
			}
			if previous, seen := aliases[name]; seen && previous != target {
				return fmt.Errorf("models.aliases defines %q twice (%q and %q)", name, previous, target)
			}
			aliases[name] = target
		}
		for _, name := range sortedAliasNames(aliases) {
			if chain := aliasCycle(aliases, name); chain != "" {
				return fmt.Errorf("models.aliases contains a loop: %s", chain)
			}
		}
		for _, group := range []string{cfg.Rotation.SOTA.Alias, cfg.Rotation.Sweet.Alias} {
			if group == "" {
				continue
			}
			if _, shadowed := aliases[group]; shadowed {
				return fmt.Errorf("models.aliases must not reuse the rotation alias %q; point an alias at it instead", group)
			}
		}
		cfg.Models.Aliases = aliases
	}

	hidden := make([]string, 0, len(cfg.Models.Hidden))
	seen := make(map[string]bool, len(cfg.Models.Hidden))
	for _, raw := range cfg.Models.Hidden {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		hidden = append(hidden, name)
	}
	if len(hidden) == 0 {
		hidden = nil
	}
	cfg.Models.Hidden = hidden
	return nil
}

// aliasCycle returns the chain that loops back on itself starting at start, or
// "" when the chain terminates.
func aliasCycle(aliases map[string]string, start string) string {
	seen := make(map[string]bool, len(aliases))
	chain := make([]string, 0, len(aliases)+1)
	current := start
	for {
		if seen[current] {
			return strings.Join(append(chain, current), " -> ")
		}
		seen[current] = true
		chain = append(chain, current)
		next, ok := aliases[current]
		if !ok {
			return ""
		}
		current = next
	}
}

func sortedAliasNames(aliases map[string]string) []string {
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
