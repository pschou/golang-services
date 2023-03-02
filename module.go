package main

import (
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/xanzy/go-gitlab"
)

func mod(w http.ResponseWriter, r *http.Request) {
	// find a project ID by module name
	module := mux.Vars(r)["module"]
	project, client, ok := Lookup(module)
	if !ok {
		http.NotFound(w, r)
		return
	}

	finalVersion := mux.Vars(r)["version"]

	parts := strings.Split(finalVersion, "-")
	if len(parts) != 3 {
		http.Error(w, "invalid version", http.StatusInternalServerError)
		return
	}

	// find a go.mod content from Gitlab by version name
	version := parts[2]

	content, _, err := client.RepositoryFiles.GetRawFile(project, "go.mod", &gitlab.GetRawFileOptions{
		Ref: &version,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// write go.mod in output
	io.WriteString(w, string(content))
}
