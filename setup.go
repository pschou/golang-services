package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/go-github/v50/github"
	"github.com/xanzy/go-gitlab"
	"gopkg.in/yaml.v3"
)

var (
	data       yamlParse
	configFile = flag.String("config", "config.yaml", "Config file for matching and connecting gitlab runners")
	ctx        = context.Background()
)

type yamlParse struct {
	Regexp  []yamlMatchReplace `yaml:"regexp"`
	Modules map[string]string  `yaml:"modules"`

	GitLabToken    string `yaml:"git-token"`
	GitLabURL      string `yaml:"git-url"`
	GitLabProvider string `yaml:"git-provider"`
	// Defines a Gitlab client
	gitClient interface{} //*gitlab.Client
}
type yamlMatchReplace struct {
	Match   string `yaml:"match"`
	Replace string `yaml:"replace"`
	regexp  *regexp.Regexp

	GitLabToken    string `yaml:"git-token"`
	GitLabURL      string `yaml:"git-url"`
	GitLabProvider string `yaml:"git-provider"`
	// Defines a Gitlab client
	gitClient interface{}
}

func Lookup(pkg string) (out string, git interface{}, ok bool) {
	// Do the absolute match first
	if out, ok = data.Modules[pkg]; ok {
		git = data.gitClient
		return
	}

	// Loop over all the Regexp to find a match and then execute
	for _, elm := range data.Regexp {
		if elm.regexp.MatchString(pkg) {
			if elm.gitClient != nil { // return the best non-nil match
				git = elm.gitClient
			} else {
				git = data.gitClient
			}

			if elm.Replace != "" {
				return elm.regexp.ReplaceAllString(pkg, elm.Replace), git, true
			}
			return pkg, git, true
		}
	}

	// Nothing matched, fail!
	return "", nil, false
}

func loadConfig() {
	// reading mapping from yaml file
	if *verbose {
		log.Println("Loading in config", *configFile)
	}
	cfg, err := ioutil.ReadFile(*configFile)
	if err != nil {
		log.Fatal(err)
	}

	// load config
	err = yaml.Unmarshal(cfg, &data)
	if err != nil {
		log.Fatal(err)
	}
	if *verbose {
		log.Println("Found", len(data.Modules), "exact module replacements")
		log.Println("Found", len(data.Regexp), "regexp match (and replace) module replacements")
	}

	// initialization of Gitlab client(s)
	if data.GitLabURL != "" {
		if *verbose {
			log.Println("Connecting to", data.GitLabURL)
		}
		data.gitClient = login(data.GitLabToken, data.GitLabURL, data.GitLabProvider)
		if err != nil {
			log.Fatal("Error connecting to git:", data.GitLabURL, err)
			log.Fatal(err)
		}
	}

	for i, elm := range data.Regexp {
		if *verbose {
			log.Println("Compiling regexp for", elm.Match, "->", elm.Replace)
		}
		data.Regexp[i].regexp, err = regexp.Compile(elm.Match)
		if err != nil {
			log.Fatal("Error compiling match:", elm.Match, err)
		}

		if elm.GitLabURL != "" {
			if *verbose {
				log.Println("Connecting to", elm.GitLabURL)
			}
			c := github.NewTokenClient(ctx, elm.GitLabToken)
			fmt.Println("c", c)
			data.Regexp[i].gitClient = login(elm.GitLabToken, elm.GitLabURL, elm.GitLabProvider)
		}
	}
}

func login(tok, apiurl, prov string) interface{} {
	switch prov {
	case "", "gitlab":
		c, err := gitlab.NewClient(tok, gitlab.WithBaseURL(apiurl))
		if err != nil {
			log.Fatal("Error connecting to gitlab:", apiurl, err)
		}
		return c
	case "github":
		c := github.NewTokenClient(ctx, tok)
		baseEndpoint, err := url.Parse(apiurl)
		if err != nil {
			log.Fatal("unable to parse url", apiurl)
		}
		if !strings.HasSuffix(baseEndpoint.Path, "/") {
			baseEndpoint.Path += "/"
		}
		if !strings.HasSuffix(baseEndpoint.Path, "/api/v3/") &&
			!strings.HasPrefix(baseEndpoint.Host, "api.") &&
			!strings.Contains(baseEndpoint.Host, ".api.") {
			baseEndpoint.Path += "api/v3/"
		}
		c.BaseURL = baseEndpoint
		meta, _, err := c.APIMeta(ctx)
		if err != nil {
			log.Fatal("Error querying git metadata:", apiurl, err)
		}
		if *verbose {
			fmt.Println("meta:", *meta)
		}
		return c
	}
	return nil
}
