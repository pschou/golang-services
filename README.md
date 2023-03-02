# GoLang Proxy Redirect Service

This utility is intended to listen for GoLang proxy request and redirect them
to the proper git project for handling such requests.  This is used for
building against private projects and not having to expose the GitProject to
the public domain to use the git mod commands.

The Syntax of the config.yml:
```yaml
# exact matches for replacing a request to a target (ie: locally hosted)
modules:
  company.com/package-a: gitlab.com/pkg-a
  company.com/package-b: gitlab.com/pkg-b
|
# default git credentials to use
git-token: AAAAAAAAAABBBBBBBBBBBBBCCCCCCCCCCDDDDDDD
git-url: https://gitlab.com
|
regexp:
- match: "mytest.domain.A/([^/*])"
  replace: "another.domain/a/$1"
  git-token: AAAAAAAAAABBBBBBBBBBBBBCCCCCCCCCCDDDDDDD
  git-url: https://another.domain
  # alternate domain can be substituted with a regexp match and replace
- match: "github.com.*"
  git-token: AAAAAAAAAABBBBBBBBBBBBBCCCCCCCCCCDDDDDDD
  git-url: https://github.com
  # without a replace, the original url is used with the provided token
```

Example running:
```bash
$ ./goproxy -verbose
2023/03/02 08:18:43 Loading CA certs
...
2023/03/02 08:18:43 Assigning HTTP default client
2023/03/02 08:18:43 Loading in config config.yaml
2023/03/02 08:18:43 Found 2 exact module replacements
2023/03/02 08:18:43 Found 2 regexp match (and replace) module replacements
2023/03/02 08:18:43 Connecting to https://github.com
2023/03/02 08:18:43 Compiling regexp for mytest.domain.A/([^/*]) -> another.domain/a/$1
2023/03/02 08:18:43 Compiling regexp for mytest.domain.B/([^/*]) -> another.domain/b/$1
2023/03/02 08:18:43 Listening with HTTP on :8080
```
