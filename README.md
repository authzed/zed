# zed

[![Docker Repository on Quay.io](https://quay.io/repository/authzed/zed/status "Docker Repository on Quay.io")](https://quay.io/repository/authzed/zed)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Build Status](https://github.com/authzed/zed/workflows/build/badge.svg)](https://github.com/authzed/zed/actions)
[![Mailing List](https://img.shields.io/badge/email-google%20groups-4285F4)](https://groups.google.com/g/authzed-oss)
[![Discord Server](https://img.shields.io/discord/844600078504951838?color=7289da&logo=discord "Discord Server")](https://discord.gg/jTysUaxXzM)
[![Twitter](https://img.shields.io/twitter/follow/authzed?color=%23179CF0&logo=twitter&style=flat-square)](https://twitter.com/authzed)

A command-line client for managing Authzed.

[Authzed] is a database and service that stores, computes, and validates your application's permissions.

Developers create a schema that models their permissions requirements and use a client, such as this one, to apply the schema to the database, insert data into the database, and query the data to efficiently check permissions in their applications.

zed features include:
- Unix-friendly interface for the [v0] and [v1alpha1] [Authzed APIs]
- Context switching that stores API Tokens securely in your OS keychain
- An experimental [OPA] REPL with authzed builtin functions

See [CONTRIBUTING.md] for instructions on how to contribute and perform common tasks like building the project and running tests.

[Authzed]: https://authzed.com
[v0]: https://docs.authzed.com/reference/api#authzedapiv0
[v1alpha1]: https://docs.authzed.com/reference/api#authzedapiv1alpha1
[Authzed APIs]: https://docs.authzed.com/reference/api
[OPA]: https://openpolicyagent.org
[CONTRIBUTING.md]: CONTRIBUTING.md

## Getting Started

We highly recommend following the **[Protecting Your First App]** guide to learn the latest best practice to integrate an application with Authzed.

If you're interested in examples for a specific version of the API, they can be found in their respective folders in the [examples directory].

[Protecting Your First App]: https://docs.authzed.com/guides/first-app
[examples directory]: /examples

## Basic Usage

### Installation

zed is currently packaged by as a _head-only_ [Homebrew] Formula for both macOS and Linux.

[Homebrew]: https://brew.sh

```sh
brew install --HEAD authzed/tap/zed
```

In order to upgrade, run:

```sh
brew reinstall zed
```

### Creating a context

In order to do anything useful, zed first needs a context: a Permissions System and API Token.

The `zed context` subcommand has operations for setting the current, creating, listing, deleting contexts.

`zed login` and `zed use` are aliases that make the most common commands more convenient.

```sh
zed login my_perms_system tc_zed_my_laptop_deadbeefdeadbeefdeadbeefdeadbeef
zed context list
```

At any point in time, the `ZED_PERMISSIONS_SYSTEM`, `ZED_ENDPOINT`, and `ZED_TOKEN` environment variables can be used to override their respective values in the current context.

### Modifying a Permissions System

For each type of noun used in Authzed, there is a zed subcommand:

- `zed schema`
- `zed relationship`
- `zed permission`

For example, you can read Object Definitions in a Permissions System's Schema, check permissions, and even create or delete relationships.

```sh
zed schema read
zed permission check document:firstdoc writer user:emilia
zed relationship create document:firstdoc reader user:beatrice
zed relationship delete document:firstdoc reader user:beatrice
```

### Open Policy Agent (OPA)

Experimentally, zed embeds an instance of [OPA] that supports additional builtin functions for accessing Authzed.

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
