# fasteasyjson

[![Build Status](https://github.com/vearutop/fasteasyjson/workflows/test-unit/badge.svg)](https://github.com/vearutop/fasteasyjson/actions?query=branch%3Amaster+workflow%3Atest-unit)
[![Coverage Status](https://codecov.io/gh/vearutop/fasteasyjson/branch/master/graph/badge.svg)](https://codecov.io/gh/vearutop/fasteasyjson)
[![GoDevDoc](https://img.shields.io/badge/dev-doc-00ADD8?logo=go)](https://pkg.go.dev/github.com/vearutop/fasteasyjson)
[![Time Tracker](https://wakatime.com/badge/github/vearutop/fasteasyjson.svg)](https://wakatime.com/badge/github/vearutop/fasteasyjson)
![Code lines](https://sloc.xyz/github/vearutop/fasteasyjson/?category=code)
![Comments](https://sloc.xyz/github/vearutop/fasteasyjson/?category=comments)

<!--- TODO Update README.md -->

Project template with GitHub actions for Go.

## Install

```
go install github.com/vearutop/fasteasyjson@latest
$(go env GOPATH)/bin/fasteasyjson --help
```

Or download binary from [releases](https://github.com/vearutop/fasteasyjson/releases).

### Linux AMD64

```
wget https://github.com/vearutop/fasteasyjson/releases/latest/download/linux_amd64.tar.gz && tar xf linux_amd64.tar.gz && rm linux_amd64.tar.gz
./fasteasyjson -version
```

### Macos Intel

```
wget https://github.com/vearutop/fasteasyjson/releases/latest/download/darwin_amd64.tar.gz && tar xf darwin_amd64.tar.gz && rm darwin_amd64.tar.gz
codesign -s - ./fasteasyjson
./fasteasyjson -version
```

### Macos Apple Silicon (M1, etc...)

```
wget https://github.com/vearutop/fasteasyjson/releases/latest/download/darwin_arm64.tar.gz && tar xf darwin_arm64.tar.gz && rm darwin_arm64.tar.gz
codesign -s - ./fasteasyjson
./fasteasyjson -version
```


## Usage

Create a new repository from this template, check out it and run `./run_me.sh` to replace template name with name of
your repository.
