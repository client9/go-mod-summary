# go-mod-summary
English summary of go.mod dependencies


## What is all this stuff?

Ever look at a `go.mod` and wonder what is all this stuff?


| Flag | Meaning |
|------|---------|
| `-s` | Show a summary of the top-level module/repo (About, Website, Topics, README) |
| `-m` | Show go.mod dependency summaries |
| `-i` | Include indirect dependencies (with `-m`) |
| `-no-cache` | Skip the local module cache; always fetch from GitHub |
| `-no-github` | Never call GitHub; only use the local module cache (misses skipped) |
| `-lines N` | Number of README lines per entry (default 3, 0 to disable) |

