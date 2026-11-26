package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/agent"
	"github.com/yetone/magpie/internal/provider"
)

// #7602: with the Routing groups plugin off, the Agents page shows no
// routing group: not in the model list, not in the count's chips, not in a
// picker's options. Two providers serve the same models, so magpie finds
// auto groups; there are no groups of the user's own, which is why the
// plugin can be off at all (one of those turns it back on). The groups
// magpie finds stay the core's for routing, so a bare model still resolves
// through one, and turning the plugin back on lists them again.
func TestAgentModelsHideGroupsWhenPluginOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("model = \"magpie/relay/m1\"\n"), 0o600)
	for _, id := range []string{"a", "b"} {
		if err := provider.Save(provider.Provider{ID: id, Name: strings.ToUpper(id), Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m", "n"}}); err != nil {
			t.Fatal(err)
		}
	}
	// The gateway serves, the groups plugin is off: no ServeGroups held.
	stop := provider.GateGroups(nil)
	defer stop()
	if provider.GroupsServed() {
		t.Fatal("groups served with the plugin off")
	}

	codex, err := agent.Find("codex")
	if err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	agentModelsAPI(m)
	models := func() []agentModelJSON {
		t.Helper()
		w := httptest.NewRecorder()
		m.ServeHTTP(w, httptest.NewRequest("GET", "/api/agent-models/codex", nil))
		if w.Code != 200 {
			t.Fatalf("GET %d %s", w.Code, w.Body)
		}
		var out struct {
			Models []agentModelJSON
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Models
	}
	pickGroups := func() []string {
		t.Helper()
		var out []string
		for _, f := range agentFields(codex, codex.Values()) {
			if f.Key != "model" {
				continue
			}
			for _, o := range f.Options {
				if o.Group == agent.RoutingGroups {
					out = append(out, o.Value)
				}
			}
		}
		return out
	}
	groupsListed := func() []string {
		t.Helper()
		var out []string
		for _, md := range models() {
			if md.Group == agent.RoutingGroups {
				out = append(out, md.ID)
			}
		}
		return out
	}
	countGroups := func() int {
		t.Helper()
		c := modelCount("codex")
		for _, g := range c.By {
			if g.Name == agent.RoutingGroups {
				return g.N
			}
		}
		return 0
	}

	if got := groupsListed(); len(got) != 0 {
		t.Errorf("plugin off: model list shows groups %v", got)
	}
	if n := countGroups(); n != 0 {
		t.Errorf("plugin off: count shows Routing groups %d", n)
	}
	if got := pickGroups(); len(got) != 0 {
		t.Errorf("plugin off: picker offers groups %v", got)
	}
	// the members are still listed, and a bare model still routes through
	// its found group for failover
	if got := models(); len(got) != 4 {
		t.Errorf("plugin off: model list has %d entries, want 4 members", len(got))
	}
	if g, ok := provider.GroupFor("m"); !ok || g != "group/auto-m" {
		t.Errorf("plugin off: GroupFor(m) = %q, %v, want group/auto-m, true", g, ok)
	}

	// the plugin back on lists them again
	remove := provider.ServeGroups()
	defer remove()
	if got := groupsListed(); !slices.Equal(got, []string{"auto/m", "auto/n"}) {
		t.Errorf("plugin on: model list shows groups %v, want [auto/m auto/n]", got)
	}
	if n := countGroups(); n != 2 {
		t.Errorf("plugin on: count shows Routing groups %d, want 2", n)
	}
	if got := pickGroups(); len(got) == 0 {
		t.Error("plugin on: picker offers no group")
	}
}
