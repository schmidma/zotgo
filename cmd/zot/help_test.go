package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func runRoot(args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	root := rootCommand()
	root.Writer = &stdout
	root.ErrWriter = &stderr
	err := root.Run(context.Background(), append([]string{"zot"}, args...))
	return stdout.String(), stderr.String(), err
}

func TestRootHelpRoutesDiscovery(t *testing.T) {
	stdout, stderr, err := runRoot("--help")
	if err != nil {
		t.Fatalf("help: %v\nstderr: %s", err, stderr)
	}
	for _, want := range []string{
		"Start with `zot doctor`",
		"zot <command> --help",
		"Get started:",
		"Find and inspect:",
		"Item details:",
		"Organize collections:",
		"Modify (local only):",
		"doctor",
		"search",
		"show",
		"attachment",
		"relation",
		"annotation",
		"note",
		"collections",
		"export",
		"item",
		"collection",
		"tag",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("root help missing %q:\n%s", want, stdout)
		}
	}
}

func TestDiscoveryTracksEveryTopLevelCommand(t *testing.T) {
	expected := map[string]string{
		"doctor":      "Get started",
		"list":        "Find and inspect",
		"show":        "Find and inspect",
		"search":      "Find and inspect",
		"collections": "Find and inspect",
		"stats":       "Find and inspect",
		"attachment":  "Item details",
		"relation":    "Item details",
		"annotation":  "Item details",
		"note":        "Item details",
		"export":      "Export",
		"item":        "Modify (local only)",
		"collection":  "Organize collections",
		"tag":         "Modify (local only)",
	}
	commands := rootCommand().Commands
	if len(commands) != len(expected) {
		t.Fatalf("top-level command count = %d, want %d", len(commands), len(expected))
	}
	for _, command := range commands {
		category, ok := expected[command.Name]
		if !ok {
			t.Errorf("unexpected top-level command %q", command.Name)
			continue
		}
		if command.Category != category {
			t.Errorf("command %q category = %q, want %q", command.Name, command.Category, category)
		}
		delete(expected, command.Name)
	}
	for name := range expected {
		t.Errorf("missing top-level command %q", name)
	}
}

func TestEveryCommandRendersHelp(t *testing.T) {
	var paths [][]string
	root := rootCommand()
	var visit func([]string, []*cli.Command)
	visit = func(prefix []string, commands []*cli.Command) {
		for _, command := range commands {
			path := append(append([]string(nil), prefix...), command.Name)
			paths = append(paths, path)
			visit(path, command.Commands)
		}
	}
	visit(nil, root.Commands)

	for _, path := range paths {
		path := path
		t.Run(strings.Join(path, "/"), func(t *testing.T) {
			stdout, stderr, err := runRoot(append(path, "--help")...)
			if err != nil {
				t.Fatalf("help: %v\nstderr: %s", err, stderr)
			}
			if !strings.Contains(stdout, "USAGE:") || !strings.Contains(stdout, "zot "+strings.Join(path, " ")) {
				t.Fatalf("help does not identify command path %q:\n%s", strings.Join(path, " "), stdout)
			}
		})
	}
}

func TestHelpSuggestsCommandsAndFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"top-level command", []string{"serach"}, "search"},
		{"nested command", []string{"item", "crate"}, "create"},
		{"leaf flag", []string{"search", "--limt", "1", "query"}, `Did you mean "--limit"?`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, err := runRoot(test.args...)
			if err == nil {
				t.Fatal("invalid invocation succeeded")
			}
			combined := stdout + stderr + err.Error()
			if !strings.Contains(combined, test.want) {
				t.Fatalf("output missing suggestion %q:\n%s", test.want, combined)
			}
		})
	}
}

func TestMissingArgumentsPointToLeafHelp(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"search"}, "zot search --help"},
		{[]string{"show"}, "zot show --help"},
		{[]string{"attachment", "show"}, "zot attachment show --help"},
		{[]string{"attachment", "import"}, "zot attachment import --help"},
		{[]string{"attachment", "import", "--parent", "PARENT01"}, "zot attachment import --help"},
		{[]string{"relation", "list"}, "zot relation list --help"},
		{[]string{"annotation", "list"}, "zot annotation list --help"},
		{[]string{"note", "list"}, "zot note list --help"},
		{[]string{"note", "get"}, "zot note get --help"},
		{[]string{"export"}, "zot export --help"},
		{[]string{"item", "template"}, "zot item template --help"},
		{[]string{"item", "patch"}, "zot item patch --help"},
		{[]string{"item", "delete"}, "zot item delete --help"},
		{[]string{"collection", "path"}, "zot collection path --help"},
		{[]string{"collection", "create"}, "zot collection create --help"},
		{[]string{"collection", "rename"}, "zot collection rename --help"},
		{[]string{"collection", "delete"}, "zot collection delete --help"},
		{[]string{"tag", "add"}, "zot tag add --help"},
		{[]string{"tag", "remove"}, "zot tag remove --help"},
		{[]string{"tag", "delete"}, "zot tag delete --help"},
	}
	for _, test := range tests {
		name := strings.Join(test.args, "/")
		t.Run(name, func(t *testing.T) {
			_, _, err := runRoot(test.args...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want help pointer %q", err, test.want)
			}
		})
	}
}

func TestHighCostHelpExplainsRouting(t *testing.T) {
	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"search", "--help"}, []string{"--limit 0", "--everything"}},
		{[]string{"attachment", "--help"}, []string{"attachment show", "attachment import", "local-only"}},
		{[]string{"note", "--help"}, []string{"note list", "note get", "rich HTML"}},
		{[]string{"collections", "--help"}, []string{"collection hierarchy", "zot collection --help"}},
		{[]string{"collection", "--help"}, []string{"zot collections", "local writes"}},
		{[]string{"item", "--help"}, []string{"Create from JSON", "--dry-run", "--yes"}},
		{[]string{"tag", "--help"}, []string{"every item", "--dry-run", "--yes"}},
	}
	for _, test := range tests {
		name := strings.Join(test.args[:len(test.args)-1], "/")
		t.Run(name, func(t *testing.T) {
			stdout, stderr, err := runRoot(test.args...)
			if err != nil {
				t.Fatalf("help: %v\nstderr: %s", err, stderr)
			}
			for _, want := range test.want {
				if !strings.Contains(stdout, want) {
					t.Errorf("help missing %q:\n%s", want, stdout)
				}
			}
		})
	}
}
