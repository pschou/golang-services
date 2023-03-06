package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
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
	Modules map[string]string  `yaml:"modules"`
	Regexp  []yamlMatchReplace `yaml:"regexp"`

	GitLabToken    string `yaml:"git-token"`
	GitLabURL      string `yaml:"git-url"`
	GitLabProvider string `yaml:"git-provider"`
	//GitLabBase     string `yaml:"git-base"`
	// Defines a Gitlab client
	gitClient interface{} //*gitlab.Client

	LocalCache string `yaml:"local-cache"`
}
type yamlMatchReplace struct {
	Match  string `yaml:"match"`
	Base   string `yaml:"base"`
	Group  string `yaml:"group"`
	Repo   string `yaml:"repo"`
	regexp *regexp.Regexp

	GitLabToken    string `yaml:"git-token"`
	GitLabURL      string `yaml:"git-url"`
	GitLabProvider string `yaml:"git-provider"`
	//GitLabBase     string `yaml:"git-base"`
	// Defines a Gitlab client
	gitClient interface{}
}

type cacheEntry struct {
	dir, path, ver, date, sha string
}

func checkCache(module, version string) *cacheEntry {
	//fmt.Println("checking cache", module, version)
	entries, err := os.ReadDir(path.Join(data.LocalCache, module))
	if err != nil {
		return nil
	}
	var f cacheEntry
	for _, e := range entries {
		name := e.Name()
		//fmt.Println(e.Name())
		if !strings.HasSuffix(name, ".tgz") || len(name) < 59 {
			continue
		}
		dp := len(name) - 59
		f.ver = name[:dp]           // get version (if any)
		f.date = name[dp : dp+14]   // get the date portion
		f.sha = name[dp+17 : dp+55] // get sha portion
		if f.ver == version || strings.HasPrefix(version, "v0.0.0-"+f.date+"-"+f.sha[:6]) || strings.HasPrefix(version, f.sha[:12]) {
			f.path = path.Join(data.LocalCache, module, name)
			f.dir = path.Join(data.LocalCache, module)
			//fmt.Println("returned path", f.path, f.dir)
			return &f
		}
	}
	//fmt.Printf("failed checking cache for %s %s %#v\n", module, version, f)
	return nil
}

type lookupResult struct {
	orig                                string
	base, group, repo, path, majorVer   string
	baseGroupRepo, groupRepo, cleanPath string
	git                                 interface{}
}

func Lookup(pkg string) (lr *lookupResult, ok bool) {
	// Switch the !c to a uppercase letter for safe url -> git repos
	for i := strings.Index(pkg, "!"); i >= 0 && i+1 < len(pkg); i = strings.Index(pkg, "!") {
		pkg = pkg[:i] + strings.ToUpper(pkg[i+1:i+2]) + pkg[i+2:]
	}

	// Do the absolute match first for references
	lr = &lookupResult{orig: pkg}
	var out string
	if out, ok = data.Modules[pkg]; ok {
		if *verbose {
			fmt.Println("switching from", pkg, "to", out)
		}
		pkg = out
	}

	if data.gitClient != nil {
		lr.git, ok = data.gitClient, true
		parts := strings.SplitN(pkg, "/", 4)
		switch len(parts) {
		case 1, 2:
		case 3:
			lr.base, lr.group, lr.repo = parts[0], parts[1], parts[2]
		default:
			lr.base, lr.group, lr.repo, lr.path, lr.cleanPath = parts[0], parts[1], parts[2], parts[3], parts[3]
			{ // Split out a major version -- if there is one, first the start
				verParts := strings.SplitN(parts[3], "/", 2)
				if len(verParts[0]) > 1 && verParts[0][0] == 'v' {
					if _, err := strconv.ParseInt(verParts[0][1:], 10, 32); err == nil {
						if len(verParts) > 1 {
							lr.majorVer, lr.cleanPath = verParts[0], verParts[1]
						} else {
							lr.majorVer, lr.cleanPath = verParts[0], ""
						}
					}
				}
			}
			/*
				if lr.majorVer == "" { // then the end
					fol, ver := path.Split(parts[3])
					fmt.Println("fol=", fol, "ver=", ver)
					if len(ver) > 1 && ver[0] == 'v' {
						if _, err := strconv.ParseInt(ver[1:], 10, 32); err == nil {
							lr.majorVer, lr.cleanPath = ver, fol
						}
					}
				}*/
		}
	}

	// Loop over all the Regexp to find a match and then execute
	for _, elm := range data.Regexp {
		if elm.regexp.MatchString(pkg) {
			ok = true
			if elm.gitClient != nil { // return the best non-nil match
				lr.git = elm.gitClient
			} else {
				lr.git = data.gitClient
			}

			if elm.Base != "" {
				lr.base = elm.regexp.ReplaceAllString(pkg, elm.Base)
			}
			if elm.Group != "" {
				lr.group = elm.regexp.ReplaceAllString(pkg, elm.Group)
			}
			if elm.Repo != "" {
				lr.repo = elm.regexp.ReplaceAllString(pkg, elm.Repo)
			}
			if p := strings.SplitN(lr.repo, "/", 2); len(p) > 1 {
				lr.repo, lr.path = p[0], p[1]
			}

			if *verbose {
				fmt.Println("in", pkg, "to b:", lr.base, "g:", lr.group, "r:", lr.repo)
			}
			break
		}
	}
	//	if lr.majorVer == "" {
	lr.groupRepo = path.Join(lr.group, lr.repo)
	lr.baseGroupRepo = path.Join(lr.base, lr.group, lr.repo)

	if *verbose {
		fmt.Printf("lr = %#v\n", lr)
	}
	return
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
		adat, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(adat))

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
			log.Println("Compiling regexp for", elm.Match)
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
			if *verbose {
				fmt.Printf("github client: %#v\n", c)
			}
			data.Regexp[i].gitClient = login(elm.GitLabToken, elm.GitLabURL, elm.GitLabProvider)
		}
	}
}

func login(tok, apiurl, prov string) interface{} {
	switch prov {
	case "offline":
		return struct{}{}
	case "gitlab":
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
		//meta, _, err := c.APIMeta(ctx)
		//if err != nil {
		//	log.Fatal("Error querying git metadata:", apiurl, err)
		//}
		//if *verbose {
		//	fmt.Println("meta:", *meta)
		//}
		return c
	default:
		log.Fatal("Unknown provider ", prov, " for ", apiurl)
	}
	return nil
}
