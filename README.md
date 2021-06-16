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

### Authenticating with a Permissions System

In order to interact with a Permissions System, zed first needs an API token.
zed stores API Tokens in your OS's keychain; all other non-sensitive data is stored in `$XDG_CONFIG_HOME/zed` with a fallback of `$HOME/.zed`.
The environment variables `$ZED_PERMISSIONS_SYSTEM`, `$ZED_ENDPOINT`, and `$ZED_TOKEN` can be used to override their respective values in the current context.

```sh
$ zed token save my_perms_system tu_zed_my_laptop_deadbeefdeadbeefdeadbeefdeadbeef
$ zed token use  my_perms_system # `token save` does this, but we'll be explicit
$ zed token list
USING	PERMISSIONS SYSTEM	ENDPOINT            	TOKEN
  ✓  	my_perms_system   	grpc.authzed.com:443	tu_zed_my_laptop_<redacted>
```

### Schemas

The `schema read` command prints a tree view of the specified Object Definition(s) in a Permissions System's Schema.

```sh
$ zed schema read document
document
 ├── writer
 └── reader
      └── union
           ├── _this
           └── TUPLE_OBJECT: writer
```

### Relationships

Once a Permissions System has a Schema that defines Relations and Permissions for its Objects, it can be populated with Relationships -- think of them like unique rows in a database.
Relationship updates always yield a new Zed Token, which can be optionally provided to improve performance on Permissions operations.

```sh
$ zed relationship create user:emilia writer document:firstdoc
CAESAwiLBA==

$ zed relationship delete user:beatrice writer document:firstdoc
CAESAwiMBA==

$ zed relationship create user:beatrice reader document:firstdoc
CAESAwiMBA==
```

### Permissions

After there are Relationships within a Permissions System, you can start performing operations on Permissions.

The `permission check` command determines whether or not the Subject has a Permission on a particular Object.

```sh
$ zed permission check user:emilia writer document:firstdoc
true

$ zed permission check user:emilia reader document:firstdoc
true

$ zed permission check user:beatrice reader document:firstdoc
true

$ zed permission check user:beatrice writer document:firstdoc
false
```

The `permission expand` command provides a tree view of the expanded structure of a particular Permission.

```
$ zed permission expand document:firstdoc reader
document:firstdoc->reader
 └── union
      ├── user:beatrice
      └── document:firstdoc->writer
           └── user:emilia
```

### Misc

For ease of scripting, most commands when piped or provided the `--json` flag have their API responses are converted into JSON.

```
$ zed schema read document | jq '.config.relation[0].name'
"writer"
```
