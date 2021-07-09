# zed

[![GoDoc](https://godoc.org/github.com/authzed/zed?status.svg)](https://godoc.org/github.com/authzed/zed)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
![Lines of Code](https://tokei.rs/b1/github/authzed/zed)
[![Discord Server](https://img.shields.io/discord/844600078504951838?color=7289da&logo=discord "Discord Server")](https://discord.gg/jTysUaxXzM)
[![Build Status](https://github.com/authzed/zed/workflows/build/badge.svg)](https://github.com/authzed/zed/actions)
[![Docker Repository on Quay.io](https://quay.io/repository/authzed/zed/status "Docker Repository on Quay.io")](https://quay.io/repository/authzed/zed)

A client for managing [authzed] or any API-compatible system from your command line.

Features included:
- Credential management
- Unix interface for the v0 API
- An extended version of [OPA] with authzed builtins

[authzed]: https://authzed.com
[OPA]: https://openpolicyagent.org

## Installation

zed is currently packaged by as a _head-only_ [Homebrew] Formula for both macOS and Linux.

[Homebrew]: https://brew.sh

```sh
$ brew install --HEAD authzed/tap/zed
```

## Example Usage

### Authenticating with a Permissions System

In order to interact with a Permissions System, zed first needs a context: a permissions system and API token.
zed stores API Tokens in your OS's keychain; all other non-sensitive data is stored in `$XDG_CONFIG_HOME/zed` with a fallback of `$HOME/.zed`.

The `zed context` command has operations for setting the current, creating, listing, deleting contexts.
`zed login` and `zed use` are aliases that make the most common commands more convenient.

The environment variables `ZED_PERMISSIONS_SYSTEM`, `ZED_ENDPOINT`, and `ZED_TOKEN` can be used to override their respective values in the current context.

```sh
$ zed login my_perms_system tc_zed_my_laptop_deadbeefdeadbeefdeadbeefdeadbeef
$ zed context list
CURRENT	PERMISSIONS SYSTEM	ENDPOINT            	TOKEN
   ✓   	my_perms_system   	grpc.authzed.com:443	tc_zed_my_laptop_<redacted>
```

### Schemas

The `schema read` command prints the specified Object Definition(s) in a Permissions System's Schema.

```sh
$ zed schema read user document
definition my_perms_system/user {}
definition my_perms_system/document {
	relation write: my_perms_system/user
	relation read: my_perms_system/user

	permission writer = write
	permission reader = read + writer
}
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

```sh
$ zed permission expand document:firstdoc reader
document:firstdoc->reader
 └── union
      ├── user:beatrice
      └── document:firstdoc->writer
           └── user:emilia
```

### Open Policy Agent (OPA)

Experimentally, zed embeds an instance of [OPA] that supports additional builtins specifically for accessing Authzed.

The following functions have been added:

```rego
authzed.check("subject:id", "permission", "object:id", "zedtoken")
```

It can be found under the `zed experiment opa` command:

```sh
$ zed experiment opa eval 'authzed.check("user:emilia", "reader", "document:firstdoc", "")'
{
  "result": [
    {
      "expressions": [
        {
          "value": true,
          "text": "authzed.check(\"user:emilia\", \"reader\", \"document:firstdoc\", \"\")",
          "location": {
            "row": 1,
            "col": 1
          }
        }
      ]
    }
  ]
}
```

