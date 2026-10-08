package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"testing"

	"github.com/flosch/pongo2/v6"
)

//go:embed templates/*
var testTemplatesFS embed.FS

func init() {
	pongo2.RegisterFilter("tojson", func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		b, err := json.Marshal(in.Interface())
		if err != nil {
			return pongo2.AsValue(""), nil
		}
		return pongo2.AsSafeValue(string(b)), nil
	})
}

func TestTemplatesCompile(t *testing.T) {
	subFS, err := fs.Sub(testTemplatesFS, "templates")
	if err != nil {
		t.Fatalf("failed to sub FS: %v", err)
	}

	set := pongo2.NewSet("test", pongo2.NewFSLoader(subFS))

	templates := []string{
		"base.html",
		"feed.html",
		"chore.html",
		"dashboard.html",
		"join.html",
		"leaderboard.html",
		"manage.html",
		"profile.html",
		"templates.html",
		"user.html",
	}

	for _, name := range templates {
		tpl, err := set.FromFile(name)
		if err != nil {
			t.Errorf("failed to compile template %s: %v", name, err)
			continue
		}
		// Try a basic render
		out, err := tpl.Execute(pongo2.Context{
			"active":  "feed",
			"task_id": 1,
			"user_id": "123",
		})
		if err != nil {
			t.Errorf("failed to execute template %s: %v", name, err)
		}
		if len(out) == 0 {
			t.Errorf("empty output for template %s", name)
		}
	}
}
