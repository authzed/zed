# zed

[![Docker Repository on Quay.io](https://quay.io/repository/authzed/zed/status "Docker Repository on Quay.io")](https://quay.io/repository/authzed/zed)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Build Status](https://github.com/authzed/zed/workflows/build/badge.svg)](https://github.com/authzed/zed/actions)
[![Mailing List](https://img.shields.io/badge/email-google%20groups-4285F4)](https://groups.google.com/g/authzed-oss)
[![Discord Server](https://img.shields.io/discord/844600078504951838?color=7289da&logo=discord "Discord Server")](https://discord.gg/jTysUaxXzM)
[![Twitter](https://img.shields.io/twitter/follow/authzed?color=%23179CF0&logo=twitter&style=flat-square)](https://twitter.com/authzed)

A command-line client for managing [SpiceDB] and [Authzed].

zed features include:
- Unix-friendly interface for the [v1] Authzed [API]
- Context switching that stores credentials securely in your OS keychain
- An experimental [OPA] REPL with additional builtins for checking permissions

See [CONTRIBUTING.md] for instructions on how to contribute and perform common tasks like building the project and running tests.

[SpiceDB]: https://github.com/authzed/spicedb
[Authzed]: https://authzed.com
[v1]: https://buf.build/authzed/api/docs/main/authzed.api.v1
[API]: https://docs.authzed.com/reference/api
[OPA]: https://openpolicyagent.org
[CONTRIBUTING.md]: CONTRIBUTING.md

## Getting Started

### Follow the Guide

We highly recommend following the **[Protecting Your First App]** guide to learn the latest best practice to integrate an application with Authzed.

[Protecting Your First App]: https://docs.authzed.com/guides/first-app

### Installation

zed is currently packaged by [Homebrew] for both macOS and Linux.
Individual releases are also available on the [releases page].

[Homebrew]: https://brew.sh
[releases page]: https://github.com/authzed/zed/releases

```sh
brew install authzed/tap/zed
```

### Creating a context

In order to do anything useful, zed first needs a context: a named pair of the endpoint and its accompanying credential.

The `zed context` subcommand has operations for setting the current, creating, listing, deleting contexts.

```sh
zed context set prod grpc.authzed.com:443 tc_zed_my_laptop_deadbeefdeadbeefdeadbeefdeadbeef
zed context set dev localhost:80 testpresharedkey
zed context list
```

At any point in time, the `ZED_ENDPOINT` and `ZED_TOKEN` environment variables can be used to override their respective values in the current context.


### Viewing & modifying data

For each type of noun used in SpiceDB, there is a zed subcommand:

- `zed schema`
- `zed relationship`
- `zed permission`

For example, you can read a schema, check permissions, and create or delete relationships:

```sh
zed schema read
zed permission check document:firstdoc writer user:emilia
zed relationship create document:firstdoc reader user:beatrice
zed relationship delete document:firstdoc reader user:beatrice
```

### Open Policy Agent (OPA)

Experimentally, zed embeds an instance of [OPA] that supports additional builtin functions for accessing SpiceDB.

The following functions have been added:

```rego
authzed.check("resource:id", "permission", "subject:id", "zedtoken")
```

It can be found under the `zed experiment opa` command:

```sh
$ zed experiment opa eval 'authzed.check("document:firstdoc", "reader", "user:emilia", "")'
{
  "result": [
    {
      "expressions": [
        {
          "value": true,
          "text": "authzed.check(\"document:firstdoc\", \"reader\", \"user:emilia\", \"\")",
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

If you are interested in OPA, please feel free to [reach out] to provide feedback.

[reach out]: https://authzed.com/contact/
