package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/go-github/v50/github"
	"github.com/gorilla/mux"
	"github.com/xanzy/go-gitlab"
)

func list(w http.ResponseWriter, r *http.Request) {
	if *verbose {
		log.Printf("Got list request: %#v", r)
	}
	// find a project ID by module name
	module := mux.Vars(r)["module"]
	project, myClient, ok := Lookup(module)
	//fmt.Println("found", project, client, ok)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch client := myClient.(type) {
	case *gitlab.Client:
		//fmt.Println("looking up releases")
		releases, _, err := client.Releases.ListReleases(project,
			&gitlab.ListReleasesOptions{ListOptions: gitlab.ListOptions{PerPage: 10}})
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

		commits, _, err := client.Commits.ListCommits(project,
			&gitlab.ListCommitsOptions{ListOptions: gitlab.ListOptions{PerPage: 10}})
		for _, commit := range commits {
			// build output
			fmt.Fprintf(w,
				"v0.0.0-%s-%s\n", // v0.0.0 is only if there is no tag, otherwise tag name should be added
				commit.CommittedDate.Format("20060102150405"),
				commit.ID[0:12],
			)
		}
		//fmt.Printf("commits: %s %#v\n", err, commits)
	case *github.Client:
		parts := strings.SplitN(project, "/", 3)
		if len(parts) == 1 {
			http.Error(w, "Invalid project: "+project, http.StatusInternalServerError)
			return
		}
		releases, _, _ := client.Repositories.ListTags(ctx, parts[0], parts[1],
			&github.ListOptions{PerPage: 10})
		if len(releases) > 0 {
			//fmt.Printf("releases: %s %#v\n", err, releases)
			for _, entry := range releases {
				fmt.Fprintf(w, "%s\n", *entry.Name)
			}
			return
		}

		commits, _, err := client.Repositories.ListCommits(ctx, parts[0], parts[1],
			&github.CommitsListOptions{ListOptions: github.ListOptions{PerPage: 10}})
		if *verbose && err != nil {
			log.Println("error listing", err)
		}

		for _, commit := range commits {
			//adat, _ := json.MarshalIndent(commit.SHA, "", "  ")
			//fmt.Printf("sha: %s %s\n", err, adat)
			// build output
			sha := *(commit.SHA)
			fmt.Fprintf(w,
				"v0.0.0-%s-%s\n", // v0.0.0 is only if there is no tag, otherwise tag name should be added
				commit.Commit.Committer.Date.Format("20060102150405"),
				sha[0:12],
			)
		}
	}
}
