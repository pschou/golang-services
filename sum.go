package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/google/go-github/v50/github"
	"github.com/gorilla/mux"
	"github.com/pschou/go-tease"
	"github.com/xanzy/go-gitlab"
)

func sum(w http.ResponseWriter, r *http.Request) {
	if *verbose {
		log.Printf("Got archive request: %#v", r)
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

	switch client := lr.git.(type) {
	case *gitlab.Client:
		pr, pw := io.Pipe()

		var clientErr error
		go func() {
			format := "tar.gz"
			_, clientErr = client.Repositories.StreamArchive(lr.groupRepo, pw, &gitlab.ArchiveOptions{
				Format: &format,
				SHA:    &ver.Origin.Hash,
			})
			pw.Close()
		}()

		pr.Read([]byte{}) // trigger a short read
		if clientErr != nil {
			http.Error(w, clientErr.Error(), http.StatusInternalServerError)
			return
		}

		var rdr *tar.Reader
		tr := tease.NewReader(pr)

		gz, err := gzip.NewReader(tr)
		if err == nil {
			rdr = tar.NewReader(gz)
			tr.Pipe()
		} else {
			tr.Seek(0, io.SeekStart)
			rdr = tar.NewReader(tr)
			tr.Pipe()
		}

		pkg, mod := modsum(rdr, module, ver.Version)
		if pkg != "" {
			fmt.Fprintf(w, "%s %s h1:%s\n", module, ver.Version, pkg)
		}
		if mod != "" {
			fmt.Fprintf(w, "%s %s/go.mod h1:%s\n", module, ver.Version, mod)
		}

	case *github.Client:
		link, _, err := client.Repositories.GetArchiveLink(ctx, lr.group, lr.repo, github.Tarball,
			&github.RepositoryContentGetOptions{Ref: ver.Origin.Hash}, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Println("got link: ", link)

		resp, err := http.Get(link.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		var rdr *tar.Reader
		tr := tease.NewReader(resp.Body)

		gz, err := gzip.NewReader(tr)
		if err == nil {
			rdr = tar.NewReader(gz)
			tr.Pipe()
		} else {
			tr.Seek(0, io.SeekStart)
			rdr = tar.NewReader(tr)
			tr.Pipe()
		}

		pkg, mod := modsum(rdr, module, ver.Version)
		if pkg != "" {
			fmt.Fprintf(w, "%s %s h1:%s\n", module, ver.Version, pkg)
		}
		if mod != "" {
			fmt.Fprintf(w, "%s %s/go.mod h1:%s\n", module, ver.Version, mod)
		}
	}
}

type fileSum struct {
	name string
	hash hash.Hash
}

func modsum(tr *tar.Reader, module, finalVersion string) (pkg, mod string) {
	var item *tar.Header
	directory := fmt.Sprintf("%s@%s", module, finalVersion)

	var fileSums []fileSum
	var err error

	// read unpacked files into hash, with changed folder name
	for item, err = tr.Next(); err == nil; item, err = tr.Next() {
		//fmt.Println("tar item:", item)
		parts := strings.SplitN(item.Name, "/", 2)
		if len(parts) == 1 || item.Typeflag != tar.TypeReg {
			continue
		}

		fs := fileSum{
			name: directory + "/" + parts[1],
			hash: sha256.New(),
		}

		io.Copy(fs.hash, tr)
		fileSums = append(fileSums, fs)

		if parts[1] == "go.mod" {
			modHash := sha256.New()
			fmt.Fprintf(modHash, "%0x  %s\n", fs.hash.Sum(nil), "go.mod")
			mod = base64.StdEncoding.EncodeToString(modHash.Sum(nil))
		}
	}
	if err != nil && err != io.EOF {
		return
	}

	// Sort the files by name
	sort.Slice(fileSums, func(i, j int) bool {
		return strings.Compare(fileSums[i].name, fileSums[j].name) < 0
	})

	// Hash it all
	dirHash := sha256.New()
	for _, f := range fileSums {
		//fmt.Println("%0x  %s\n", f.hash.Sum(nil), f.name)
		fmt.Fprintf(dirHash, "%0x  %s\n", f.hash.Sum(nil), f.name)
	}
	pkg = base64.StdEncoding.EncodeToString(dirHash.Sum(nil))
	return
}
