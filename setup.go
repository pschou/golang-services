package main

import (
	"flag"
	"io/ioutil"
	"log"
	"regexp"

	"github.com/xanzy/go-gitlab"
	"gopkg.in/yaml.v3"
)

var (
	data       yamlParse
	configFile = flag.String("config", "config.yaml", "Config file for matching and connecting gitlab runners")
)

type yamlParse struct {
	Regexp  []yamlMatchReplace `yaml:"regexp"`
	Modules map[string]string  `yaml:"modules"`

	GitLabToken string `yaml:"git-token"`
	GitLabURL   string `yaml:"git-url"`
	// Defines a Gitlab client
	gitClient *gitlab.Client
}
type yamlMatchReplace struct {
	Match   string `yaml:"match"`
	Replace string `yaml:"replace"`
	regexp  *regexp.Regexp

	GitLabToken string `yaml:"git-token"`
	GitLabURL   string `yaml:"git-url"`
	// Defines a Gitlab client
	gitClient *gitlab.Client
}

func Lookup(pkg string) (out string, git *gitlab.Client, ok bool) {
	// Do the absolute match first
	if out, ok = data.Modules[pkg]; ok {
		git = data.gitClient
		return
	}

	// Loop over all the Regexp to find a match and then execute
	for _, elm := range data.Regexp {
		if elm.regexp.MatchString(pkg) {
			var client *gitlab.Client
			if elm.gitClient != nil { // return the best non-nil match
				client = elm.gitClient
			} else {
				client = data.gitClient
			}

			if elm.Replace != "" {
				return elm.regexp.ReplaceAllString(pkg, elm.Replace), client, true
			}
			return pkg, client, true
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
		data.gitClient, err = gitlab.NewClient(data.GitLabToken, gitlab.WithBaseURL(data.GitLabURL))
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
			data.Regexp[i].gitClient, err = gitlab.NewClient(elm.GitLabToken, gitlab.WithBaseURL(elm.GitLabURL))
			if err != nil {
				log.Fatal("Error connecting to git for match:", elm.Match, elm.GitLabURL, err)
				log.Fatal(err)
			}
		}
	}
}
