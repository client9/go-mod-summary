# go-mod-summary
English summary of go.mod dependencies


## What is all this stuff?

Ever look at a `go.mod` and wonder what is all this stuff?  `go-mod-summary` looks at the `go.mod` file and looks in your GOENVCACHE or GitHub to generate a quick summary.

It looks like this:


```
github.com/tomwright/dasel/v3
  Version: v3.3.2
  README:  Dasel
           Dasel (short for Data-Select) is a command-line tool and library for querying, modifying, and transforming data structures such as JSON, YAML, TOML, XML, and CSV.
           It provides a consistent, powerful syntax to traverse and update data — making it useful for developers, DevOps, and data wrangling tasks.

github.com/yuin/goldmark
  Version: v1.7.8
  README:  goldmark
           > A Markdown parser written in Go. Easy to extend, standards-compliant, well-structured.
           goldmark is compliant with CommonMark 0.31.2.

golang.org/x/net
  Version: v0.47.0
  README:  Go Networking
           This repository holds supplementary Go networking packages.
```


## Flags!

| Flag | Meaning |
|------|---------|
| `-s` | Show a summary of the top-level module/repo (About, Website, Topics, README) |
| `-m` | Show go.mod dependency summaries |
| `-i` | Include indirect dependencies (with `-m`) |
| `-no-cache` | Skip the local module cache; always fetch from GitHub |
| `-no-github` | Never call GitHub; only use the local module cache (misses skipped) |
| `-lines N` | Number of README lines per entry (default 3, 0 to disable) |


