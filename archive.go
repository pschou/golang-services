package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/google/go-github/v50/github"
	"github.com/gorilla/mux"
	"github.com/pschou/go-memdiskbuf"
	"github.com/pschou/go-tease"
	"github.com/xanzy/go-gitlab"
)

func archive(w http.ResponseWriter, r *http.Request) {
	if *verbose {
		log.Printf("Got archive request: %#v", r.RequestURI)
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
			writeZip(w, fh, module, lr.cleanPath, ver.Version)
			return
		}
	}

	var tr *tease.Reader
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

		tr = tease.NewReader(pr)
	case *github.Client:

		link, _, err := client.Repositories.GetArchiveLink(ctx, lr.group, lr.repo, github.Tarball,
			&github.RepositoryContentGetOptions{Ref: ver.Origin.Hash}, true)
		if err != nil {
			//log.Println("Could not get link", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if *verbose {
			fmt.Println("got link: ", link.String())
		}

		resp, err := http.Get(link.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		tr = tease.NewReader(resp.Body)
	default:
		// Client is not set
		http.Error(w, "No git client available for "+module, http.StatusInternalServerError)
		return
	}

	var rdr io.ReadSeeker
	_, err := gzip.NewReader(tr)
	if err != nil { // We have a gzip stream!
		http.Error(w, "Archive is not TGZ: "+module, http.StatusInternalServerError)
		return
	}

	tr.Seek(0, io.SeekStart)
	if ver.cachePath != "" {
		// Write the cache to disk
		os.MkdirAll(ver.cacheDir, 0755)
		if fh, err := os.Create(ver.cachePath); err == nil {
			defer fh.Close()
			tr.Pipe()
			io.Copy(fh, tr)
			fh.Seek(0, io.SeekStart)
			rdr = fh
		} else {
			if *verbose {
				log.Println("Error creating cache file:", err)
			}
			rdr = tr
		}
	} else {
		rdr = tr
	}

	// build reply zip
	writeZip(w, rdr, module, lr.cleanPath, ver.Version)
}

func writeZip(w http.ResponseWriter, r io.ReadSeeker, module, folder, finalVersion string) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tr := tar.NewReader(gz)

	// Make sure we select a folder
	if folder != "" {
		folder = folder + "/"
	}

	var ignorePaths []string

	// Get all go.mod files in the archive locations
	var item *tar.Header
	for item, err = tr.Next(); err == nil; item, err = tr.Next() {
		if parts := strings.SplitN(item.Name, "/", 2); len(parts) > 1 {
			dn, fn := path.Split(parts[1])
			if *verbose {
				fmt.Printf("name: %q dn: %q=%q fn=%q\n", item.Name, dn, folder, fn)
			}
			if fn == "go.mod" && dn != folder {
				ignorePaths = append(ignorePaths, dn)
			}
		}
	}
	if err != nil && err != io.EOF {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	//fmt.Println("rewinding")

	// Go back to the start
	r.Seek(0, io.SeekStart)
	gz, _ = gzip.NewReader(r)
	tr = tar.NewReader(gz)

	// Creates new memory buffer for our zip file
	f, err := os.CreateTemp("", "goproxy-archive")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f.Close()
	buffer := memdiskbuf.NewBuffer(f.Name(), 200<<10, 32<<10)
	defer func() {
		buffer.Reset()
		os.Remove(f.Name())
	}()
	writer := zip.NewWriter(buffer)
	directory := fmt.Sprintf("%s@%s", module, finalVersion)

	// add unpacked files into buffer, with changed folder name
zipfiles:
	for item, err = tr.Next(); err == nil; item, err = tr.Next() {
		if *verbose {
			fmt.Println("tar item:", item.Name)
		}
		parts := strings.SplitN(item.Name, "/", 2)
		if len(parts) < 2 || parts[1] == "" || hasBadName(item.Name) {
			if *verbose {
				fmt.Println("  skip empty")
			}
			continue
		}
		if !strings.HasPrefix(parts[1], folder) {
			if *verbose {
				fmt.Println("  skip has prefix", parts[1], folder)
			}
			continue
		}

		// Don't include paths with mod files
		for _, ip := range ignorePaths {
			if strings.HasPrefix(parts[1], ip) &&
				strings.HasPrefix(ip, folder) { // Look for sub folders which have alternate go.mod
				if *verbose {
					fmt.Println("  skip ip", ip)
				}
				continue zipfiles
			}
		}

		// Replace folder name with the module and verison name
		switch item.Typeflag {
		case tar.TypeReg:
			file, err := writer.CreateHeader(&zip.FileHeader{
				Name:     directory + "/" + parts[1],
				Modified: item.ModTime})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			io.Copy(file, tr)
			/*case tar.TypeDir:
			_, err := writer.CreateHeader(&zip.FileHeader{
				Name:     directory + "/" + parts[1],
				Modified: item.ModTime})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}*/
		}

	}
	if err != nil && err != io.EOF {
		log.Println("archive error", err, "for", module)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writer.Flush()
	err = writer.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// write otput of buffer
	w.Header().Set("Content-Length", strconv.FormatInt(int64(buffer.Len()), 10))
	io.Copy(w, buffer)
}

func hasBadName(path string) bool {
	parts := strings.Split(path, "/")
	for _, p := range parts {
		//if len(p) > 0 && p[0] == '.' ||
		if p == "vendor" {
			return true
		}
	}
	return false
}

/*func isGoFile(name string) bool {
*	switch {
		case name == "go.mod", name == "go.sum":
			return true
		}
		return false
}*/
