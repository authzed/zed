# zed

[![GoDoc](https://godoc.org/github.com/authzed/zed?status.svg)](https://godoc.org/github.com/authzed/zed)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
![Lines of Code](https://tokei.rs/b1/github/authzed/zed)
[![Discord Server](https://img.shields.io/discord/844600078504951838?color=7289da&logo=discord "Discord Server")](https://discord.gg/jTysUaxXzM)
[![Build Status](https://github.com/authzed/zed/workflows/build/badge.svg)](https://github.com/authzed/zed/actions)
[![Docker Repository on Quay.io](https://quay.io/repository/authzed/zed/status "Docker Repository on Quay.io")](https://quay.io/repository/authzed/zed)

A client for managing [authzed] or any API-compatible system from your command line.

[authzed]: https://authzed.com

## Installation

zed is currently packaged by as a _head-only_ [Homebrew] Formula for both macOS and Linux.

[Homebrew]: https://brew.sh

```sh
$ brew install --HEAD authzed/tap/zed
```

## Example Usage

### Logging into a permission system

In order to interact with a permission system, zed first needs an API token.
Zed stores API Tokens in the system keychain and all other non-sensitive data is stored in `$XDG_CONFIG_HOME/zed` falling back to `~/.zed`.

```
$ zed token save exampledocs tu_zed_hanazawa_deadbeefdeadbeefdeadbeefdeadbeef
NAME       	ENDPOINT            	TOKEN
exampledocs	grpc.authzed.com:443	<redacted>

$ zed token list
NAME       	ENDPOINT            	TOKEN
exampledocs	grpc.authzed.com:443	<redacted>
```


The environment variables `$ZED_SYSTEM`, `$ZED_ENDPOINT`, and `$ZED_TOKEN` can be used to override their respective values in the current context.

### Schemas

The `schema read` command prints a tree view the object definitions in a permission system.

```
$ zed schema read document
document
 ├── writer
 └── reader
      └── union
           ├── _this
           └── TUPLE_OBJECT: writer

$ zed schema read user
user
```

### Relationships

Once you've got a Schema, you fill them with Relationships -- think of them like unique rows in a database.

```
$ zed relationship create user:jimmy writer document:firstdoc
CAESAwiLBA==

$ zed relationship delete user:joey writer document:firstdoc
CAESAwiMBA==

$ zed relationship create user:joey reader document:firstdoc
CAESAwiMBA==
```

### Permissions

After there are Relationships within a Schema, you can start performing operations on Permissions.

The `permission check` command determines whether or not an Object has a particular Permission.

```
$ zed permission check user:jimmy writer document:firstdoc
true

$ zed permission check user:jimmy reader document:firstdoc
true

$ zed permission check user:joey reader document:firstdoc
true

$ zed permission check user:joey writer document:firstdoc
false
```

The `permission expand` command provides a tree view of the expanded structure of a particular Permission.

```
$ zed permission expand reader document:firstdoc
document:firstdoc reader
 └── union
      ├── user:joey
      └── document:firstdoc writer
           └── user:jimmy
```

### Misc

For ease of scripting, most commands when piped or provided the `--json` flag have their API responses are converted into JSON.

```
$ zed schema read document | jq '.config.relation[0].name'
"writer"
```
