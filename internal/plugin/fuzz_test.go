package plugin

import "testing"

// FuzzManifest: no plugin.toml makes the parser panic, and what it accepts
// has a valid name and a command.
func FuzzManifest(f *testing.F) {
	f.Add("name = \"p\"\ncommand = \"./p\"\nargs = [\"a\", 'b']\n[targets]\ndocs = \"https://x\"\nrun = [\"make\"]\n")
	f.Add("name = \"p\"\ncommand = \"x\" # c\ncapabilities = [\"pill\",]")
	f.Add("[targets]\nname = [\"\\\"\"]")
	f.Fuzz(func(t *testing.T, text string) {
		m, err := parseManifest(text)
		if err == nil && (m.Name == "" || m.Command == "") {
			t.Fatalf("accepted %+v", m)
		}
	})
}
