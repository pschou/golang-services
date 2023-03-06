package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/google/go-github/v50/github"
	"github.com/gorilla/mux"
	"github.com/xanzy/go-gitlab"
)

func mod(w http.ResponseWriter, r *http.Request) {
	if *verbose {
		log.Printf("Got module request: %#v  module: %q", r.RequestURI, mux.Vars(r)["module"])
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

	if ver.cachePath != "" { // Use cache if we got it!
		if fh, err := os.Open(ver.cachePath); err == nil {
			defer fh.Close()
			gz, err := gzip.NewReader(fh)
			if err != nil {
				log.Println("error reading gzip", ver.cachePath)
				return
			}
			tr := tar.NewReader(gz)
			var item *tar.Header
			for item, err = tr.Next(); err == nil; item, err = tr.Next() {
				if parts := strings.SplitN(item.Name, "/", 2); len(parts) > 1 {
					if dn, fn := path.Split(parts[1]); dn == lr.cleanPath && fn == "go.mod" {
						io.Copy(w, tr)
						return
					}
				}
			}
			fmt.Fprintf(w, "module %s\n", lr.orig)
			return
		}
	}

	switch client := lr.git.(type) {
	case *gitlab.Client:
		content, _, err := client.RepositoryFiles.GetRawFile(lr.groupRepo, path.Join(lr.cleanPath, "go.mod"), &gitlab.GetRawFileOptions{
			Ref: &ver.Origin.Hash,
		})
		if err != nil {
			fmt.Fprintf(w, "module %s\n", lr.baseGroupRepo)
			return
		}

		// write go.mod in output
		io.WriteString(w, string(content))
	case *github.Client:
		content, _, err := client.Repositories.DownloadContents(ctx, lr.group, lr.repo, path.Join(lr.cleanPath, "go.mod"),
			&github.RepositoryContentGetOptions{Ref: ver.Origin.Hash})
		if err != nil {
			fmt.Fprintf(w, "module %s\n", lr.orig)
			return
		}

		// write go.mod in output
		io.Copy(w, content)
		content.Close()
	}
}
