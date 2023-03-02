package main

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/google/go-github/v50/github"
	"github.com/gorilla/mux"
	"github.com/xanzy/go-gitlab"
)

func mod(w http.ResponseWriter, r *http.Request) {
	if *verbose {
		log.Printf("Got module request: %#v", r)
	}
	// find a project ID by module name
	module := mux.Vars(r)["module"]
	project, myClient, ok := Lookup(module)
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

	switch client := myClient.(type) {
	case *gitlab.Client:
		content, _, err := client.RepositoryFiles.GetRawFile(project, "go.mod", &gitlab.GetRawFileOptions{
			Ref: &version,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// write go.mod in output
		io.WriteString(w, string(content))
	case *github.Client:
		parts := strings.SplitN(project, "/", 3)
		if len(parts) == 1 {
			http.Error(w, "Invalid project: "+project, http.StatusInternalServerError)
			return
		}
		content, _, err := client.Repositories.DownloadContents(ctx, parts[0], parts[1], "go.mod",
			&github.RepositoryContentGetOptions{Ref: version})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// write go.mod in output
		io.Copy(w, content)
		content.Close()
	}
}
