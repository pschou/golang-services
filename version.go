package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
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
	lr, ok := Lookup(module)
	if !ok {
		http.NotFound(w, r)
		return
	}

	ver, notice := getVersion(lr, mux.Vars(r)["version"])
	if notice != "" {
		http.Error(w, notice, http.StatusNotFound)
		return
	}

	if *verbose {
		fmt.Printf("ver: %#v\n", ver)
	}
	json.NewEncoder(w).Encode(ver)
}

type VersionData struct {
	Version, Time string
	Origin        struct {
		VCS  string
		URL  string `json:",omitempty"`
		Ref  string `json:",omitempty"`
		Hash string
	}
	cacheDir, cachePath string
}

func getVersion(lr *lookupResult, version string) (reply VersionData, notice string) {
	var commitTime time.Time
	var commitHash string

	if data.LocalCache != "" {
		if cache := checkCache(path.Join(lr.base, lr.group, lr.repo, lr.cleanPath), version); cache != nil {
			if *verbose {
				fmt.Println("found cache")
			}
			var err error
			commitTime, err = time.ParseInLocation("20060102150405", cache.date, time.UTC)

			if err == nil {
				reply.Origin.Hash = cache.sha
				reply.Origin.VCS = "cache"
				reply.Time = commitTime.UTC().Format(time.RFC3339)
				reply.Version = cache.ver
				reply.cacheDir = cache.dir
				reply.cachePath = cache.path
				return
			}
		}
	}

	var search, versionDate string
	var isVersion bool
	if hyphen := strings.LastIndex(version, "-"); hyphen >= 20 && len(version) >= 34 && hyphen < len(version)-4 {
		isVersion = true
		search = version[hyphen+1:]
		versionDate = version[hyphen-14 : hyphen]
	} else {
		search = version
	}
	search = strings.TrimSuffix(search, "+incompatible")

	if *verbose {
		log.Println("looking up", search)
	}

	if reply.Origin.VCS == "" {
		switch client := lr.git.(type) {
		case *gitlab.Client:
			commit, _, err := client.Commits.GetCommit(lr.groupRepo, search)
			if err != nil {
				notice = fmt.Sprintf("not found: %s@%s: invalid version: unknown revision, %s",
					lr.base+"/"+lr.groupRepo, version, err)
				return
			}
			commitTime = commit.CommittedDate.UTC()
			commitHash = commit.ID
			reply.Origin.VCS = "git"

			{
				//fmt.Println("looking up tag", version)
				tag, _, _ := client.Tags.GetTag(lr.groupRepo, version)
				//adat, _ := json.MarshalIndent(tag, "", "  ")
				//fmt.Printf("commit: %s\n", adat)
				if tag != nil {
					reply.Version = tag.Name
				}
			}
			reply.Origin.URL = "https://" + lr.baseGroupRepo + ".git"
		case *github.Client:
			{
				commit, _, err := client.Repositories.GetCommit(ctx, lr.group, lr.repo, search,
					&github.ListOptions{PerPage: 1})
				if err != nil {
					notice = fmt.Sprintf("not found: %s@%s: invalid version: unknown revision, %s",
						lr.base+"/"+lr.groupRepo, version, err)
					return
				}
				if commit != nil {
					commitTime = commit.Commit.Committer.Date.UTC()
					commitHash = *(commit.SHA)
					reply.Origin.VCS = "git"
				}
			}
			//	}

			/*{ // Test for tag name
				release, _, err := client.Repositories.GetReleaseByTag(ctx, lr.group, lr.repo, version)
				if err != nil {
					log.Println("error looking up tag", version, err)
				}
				if release != nil {
					reply.Version = *release.TagName
				}
			}*/
			reply.Origin.URL = "https://" + lr.baseGroupRepo
		}
	}
	if commitHash == "" {
		notice = fmt.Sprintf("not found: %s@%s: invalid version: unknown revision",
			lr.baseGroupRepo, version)
		return
	}

	// if the hash was not used to search
	if !strings.HasPrefix(commitHash, search) {
		reply.Version = version
	}

	// build output
	date := commitTime.Format("20060102150405")
	if data.LocalCache != "" {
		reply.cacheDir = path.Join(data.LocalCache, lr.base, lr.groupRepo)
	}

	if reply.Version == "" {
		reply.Version = fmt.Sprintf(
			"v0.0.0-%s-%s", // v0.0.0 is only if there is no tag, otherwise tag name should be added
			date, commitHash[0:12],
		)
		if reply.cacheDir != "" {
			reply.cachePath = reply.cacheDir + "/" + date + "-" + commitHash + ".tgz"
		}
	} else {
		if reply.cacheDir != "" {
			reply.cachePath = reply.cacheDir + "/" + reply.Version + date + "-" + commitHash + ".tgz"
		}
		reply.Origin.Ref = "refs/tags/" + reply.Version
	}

	reply.Time = commitTime.Format(time.RFC3339)
	reply.Origin.Hash = commitHash

	// Test for date mismatch
	if isVersion && versionDate != date {
		notice = fmt.Sprintf("not found: %s@%s: invalid pseudo-version: does not match version-control timestamp (expected %s)",
			lr.baseGroupRepo, versionDate, date)
	}
	return
}
