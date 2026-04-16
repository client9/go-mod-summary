# go-mod-summary
English summary of go.mod dependencies


## What is all this stuff?

Ever look at a `go.mod` and wonder what is all this stuff?  `go-mod-summary` looks at the `go.mod` file and looks in your GOENVCACHE or GitHub to generate a quick summary.

Here's a sample:

```sh

% go-mod-summary -no-cache github.com/vale-cli/vale

github.com/tomwright/dasel/v3
  Version: v3.3.2
  About:   Select, put and delete data from JSON, TOML, YAML, XML, INI, HCL and CSV files with a single tool. Also available as a go mod.
  License: MIT
  Website: https://daseldocs.tomwright.me
  Topics:  cli, config, configuration, data-processing, data-structures, data-wrangling, devops-tools, go, golang, hcl2, json, json-processing, parser, query, selector, toml, update, xml, yaml, yaml-processor
  README:  Dasel
           Dasel (short for Data-Select) is a command-line tool and library for querying, modifying, and transforming data structures such as JSON, YAML, TOML, XML, and CSV.
           It provides a consistent, powerful syntax to traverse and update data — making it useful for developers, DevOps, and data wrangling tasks.

github.com/yuin/goldmark
  Version: v1.7.8
  About:   A markdown parser written in Go. Easy to extend, standard(CommonMark) compliant, well structured.
  License: MIT
  Topics:  commonmark, go, golang, markdown
  README:  goldmark
           > A Markdown parser written in Go. Easy to extend, standards-compliant, well-structured.
           goldmark is compliant with CommonMark 0.31.2.

golang.org/x/net
  Version: v0.47.0
  About:   Go supplementary network libraries
  License: BSD-3-Clause
  Website: https://golang.org/x/net
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


