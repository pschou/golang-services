package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/xanzy/go-gitlab"
)

func archive(w http.ResponseWriter, r *http.Request) {
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

	// get zip archive from Gitlab by version name
	version := parts[2]

	format := "zip"
	content, _, err := client.Repositories.Archive(project, &gitlab.ArchiveOptions{
		Format: &format,
		SHA:    &version,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// unpack zip archive into memory
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// creates new memory buffer
	buffer := bytes.NewBuffer([]byte{})
	writer := zip.NewWriter(buffer)

	// add unpacked files into memory buffer, with changed folder name
	for _, item := range reader.File {
		parts := strings.Split(item.Name, "/")
		if len(parts) == 0 {
			continue
		}

		// replace folder name
		directory := fmt.Sprintf("%s@%s", module, finalVersion)
		file, err := writer.Create(strings.Replace(item.Name, parts[0], directory, 1))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		closer, err := item.Open()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data, err := ioutil.ReadAll(closer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		closer.Close()

		_, err = file.Write(data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	err = writer.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// write otput of buffer
	w.Header().Set("Content-Length", strconv.FormatInt(int64(buffer.Len()), 10))
	io.WriteString(w, buffer.String())
}
