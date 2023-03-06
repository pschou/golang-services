package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/go-github/v50/github"
	"github.com/gorilla/mux"
	"github.com/xanzy/go-gitlab"
)

func latest(w http.ResponseWriter, r *http.Request) {
	var reply VersionData
	reply.Origin.VCS = "git"

	if *verbose {
		log.Printf("Got latest request: %#v", r)
	}
	// find a project ID by module name
	module := mux.Vars(r)["module"]
	lr, ok := Lookup(module)
	//fmt.Println("found", project, client, ok)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch client := lr.git.(type) {
	case *gitlab.Client:
		//fmt.Println("looking up releases")
		releases, _, err := client.Releases.ListReleases(lr.groupRepo,
			&gitlab.ListReleasesOptions{ListOptions: gitlab.ListOptions{PerPage: 1}})
		//fmt.Println("err: ", err)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(releases) > 0 {
			//fmt.Printf("releases: %s %#v\n", err, releases)
			for _, entry := range releases {
				fmt.Fprintf(w, "%s\n", entry.TagName)
			}
			return
		}

		commits, _, err := client.Commits.ListCommits(lr.groupRepo,
			&gitlab.ListCommitsOptions{ListOptions: gitlab.ListOptions{PerPage: 1}})

		if len(commits) == 1 {
			commit := commits[0]
			// build output
			reply.Version = fmt.Sprintf(
				"v0.0.0-%s-%s", // v0.0.0 is only if there is no tag, otherwise tag name should be added
				commit.CommittedDate.UTC().Format("20060102150405"),
				commit.ID[0:12],
			)
			reply.Origin.URL = "https://" + lr.baseGroupRepo + ".git"
			reply.Time = commit.CommittedDate.UTC().Format(time.RFC3339)
			reply.Origin.Hash = commit.ID
		}
		//fmt.Printf("commits: %s %#v\n", err, commits)
	case *github.Client:
		releases, _, _ := client.Repositories.ListTags(ctx, lr.group, lr.repo,
			&github.ListOptions{PerPage: 1})
		if len(releases) > 0 {
			//fmt.Printf("releases: %s %#v\n", err, releases)
			for _, entry := range releases {
				fmt.Fprintf(w, "%s\n", *entry.Name)
			}
			return
		}

		commits, _, err := client.Repositories.ListCommits(ctx, lr.group, lr.repo,
			&github.CommitsListOptions{ListOptions: github.ListOptions{PerPage: 1}})
		if *verbose && err != nil {
			log.Println("error listing", err)
		}

		// build output
		if len(commits) == 1 {
			commit := commits[0]
			sha := *(commit.SHA)
			reply.Version = fmt.Sprintf(
				"v0.0.0-%s-%s", // v0.0.0 is only if there is no tag, otherwise tag name should be added
				commit.Commit.Committer.Date.UTC().Format("20060102150405"),
				sha[0:12],
			)
			reply.Origin.URL = "https://" + lr.baseGroupRepo
			reply.Time = commit.Commit.Committer.Date.UTC().Format(time.RFC3339)
			reply.Origin.Hash = sha
		}
	}
	json.NewEncoder(w).Encode(reply)

}
