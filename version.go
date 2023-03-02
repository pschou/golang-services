package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v50/github"
	"github.com/gorilla/mux"
	"github.com/xanzy/go-gitlab"
)

func version(w http.ResponseWriter, r *http.Request) {
	if *verbose {
		log.Printf("Got version request: %#v", r)
	}
	// find a project ID by module name
	module := mux.Vars(r)["module"]
	project, myClient, ok := Lookup(module)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var reply VersionData
	reply.Origin.VCS = "git"
	reply.Origin.URL = "https://" + module
	// find a commit from Gitlab by version name
	version := mux.Vars(r)["version"]

	parts := strings.Split(version, "-")
	if len(parts) == 3 && len(parts[1]) == 14 { // Case where we have a tagged version
		version = parts[2]
	}

	switch client := myClient.(type) {
	case *gitlab.Client:
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

		reply.Version = finalVersion
		reply.Time = commit.CommittedDate.Format(time.RFC3339)
		reply.Origin.Hash = commit.ID
		json.NewEncoder(w).Encode(reply)
	case *github.Client:
		parts := strings.SplitN(project, "/", 3)
		if len(parts) == 1 {
			http.Error(w, "Invalid project: "+project, http.StatusInternalServerError)
			return
		}
		commit, _, err := client.Repositories.GetCommit(ctx, parts[0], parts[1], version,
			&github.ListOptions{PerPage: 10})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// build output
		sha := *(commit.SHA)
		finalVersion := fmt.Sprintf(
			"v0.0.0-%s-%s", // v0.0.0 is only if there is no tag, otherwise tag name should be added
			commit.Commit.Committer.Date.Format("20060102150405"),
			sha[0:12],
		)

		reply.Version = finalVersion
		reply.Time = commit.Commit.Committer.Date.Format(time.RFC3339)
		reply.Origin.Hash = sha
		json.NewEncoder(w).Encode(reply)
	}
}

type VersionData struct {
	Version, Time string
	Origin        struct {
		VCS, URL, Hash string
	}
}
