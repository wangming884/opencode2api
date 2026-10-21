package admin

import (
	"reflect"
	"testing"

	"opencode2api/internal/config"
)

// An update that omits the model alias fields must not clear them: a page
// built before the fields existed, or a script that only knows the older
// shape, would otherwise wipe a mapping the operator configured.
func TestMergeModelsUpdateKeepsOmittedFields(t *testing.T) {
	current := config.ModelsConfig{
		RefreshSeconds: 300,
		Protocols:      map[string]string{"a": "chat"},
		Aliases:        map[string]string{"gpt-4o": "glm-5.3"},
		Hidden:         []string{"glm-5.3"},
		FreeOnly:       true,
	}
	update := config.ModelsConfig{RefreshSeconds: 120, Protocols: map[string]string{"b": "responses"}}

	got := mergeModelsUpdate(current, update)
	if !reflect.DeepEqual(got.Aliases, current.Aliases) {
		t.Errorf("aliases = %v, want them kept as %v", got.Aliases, current.Aliases)
	}
	if !reflect.DeepEqual(got.Hidden, current.Hidden) {
		t.Errorf("hidden = %v, want it kept as %v", got.Hidden, current.Hidden)
	}
	if got.RefreshSeconds != 120 || !reflect.DeepEqual(got.Protocols, update.Protocols) {
		t.Errorf("fields the update did send were not taken: %+v", got)
	}
}

// An explicit empty object or array means "clear it", which is what the
// console sends when the operator empties the box.
func TestMergeModelsUpdateHonoursExplicitClear(t *testing.T) {
	current := config.ModelsConfig{
		Aliases: map[string]string{"gpt-4o": "glm-5.3"},
		Hidden:  []string{"glm-5.3"},
	}
	update := config.ModelsConfig{Aliases: map[string]string{}, Hidden: []string{}}

	got := mergeModelsUpdate(current, update)
	if len(got.Aliases) != 0 {
		t.Errorf("aliases = %v, want them cleared", got.Aliases)
	}
	if len(got.Hidden) != 0 {
		t.Errorf("hidden = %v, want it cleared", got.Hidden)
	}
}
