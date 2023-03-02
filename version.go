package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func version(w http.ResponseWriter, r *http.Request) {
	// find a project ID by module name
	module := mux.Vars(r)["module"]
	project, client, ok := Lookup(module)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// find a commit from Gitlab by version name
	version := mux.Vars(r)["version"]

	commit, _, err := client.Commits.GetCommit(project, version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// build output
	finalVersion := fmt.Sprintf(
		"v0.0.0-%s-%s", // v0.0.0 is only if there is no tag, otherwise tag name should be added
		commit.CommittedDate.Format("20060102150405"),
		commit.ID[0:12],
	)

	json.NewEncoder(w).Encode(map[string]string{
		"Version": finalVersion,
		"Time":    commit.CommittedDate.Format(time.RFC3339),
	})
}
