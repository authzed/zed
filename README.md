# zed

<!-- Uncomment when we setup CI
[![Build Status](https://github.com/authzed/zed/workflows/CI/badge.svg)](https://github.com/authzed/zed/actions)
[![Docker Repository on Quay.io](https://quay.io/repository/authzed/zed/status "Docker Repository on Quay.io")](https://quay.io/repository/authzed/zed)
-->
[![Go Report Card](https://goreportcard.com/badge/github.com/authzed/zed)](https://goreportcard.com/report/github.com/authzed/zed)
[![GoDoc](https://godoc.org/github.com/authzed/zed?status.svg)](https://godoc.org/github.com/authzed/zed)
![Lines of Code](https://tokei.rs/b1/github/authzed/zed)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![IRC Channel](https://img.shields.io/badge/freenode-%23authzed-blue.svg "IRC Channel")](http://webchat.freenode.net/?channels=authzed)

A client for managing [authzed] or any API-compatible system from your command line.

[authzed]: https://authzed.com

## Installation

zed is currently packaged by as a _head-only_ [Homebrew] Formula for both macOS and Linux.

[Homebrew]: https://brew.sh

```sh
$ brew install --HEAD authzed/tap/zed
```

## Example Usage

### Managing credentials

Configuring credentials is similar to [kubeconfig] in [kubectl].

[kubeconfig]: https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/
[kubectl]: https://kubernetes.io/docs/reference/kubectl/overview/

API Tokens are stored in the system keychain.

```
$ zed config set-token jimmy@authzed.com tu_zed_hanazawa_deadbeefdeadbeefdeadbeefdeadbeef
NAME             	ENDPOINT            	TOKEN
jimmy@authzed.com	grpc.authzed.com:443	<redacted>

$ zed config get-tokens
NAME             	ENDPOINT            	TOKEN
jimmy@authzed.com	grpc.authzed.com:443	<redacted>
```

Context data is stored in `$XDG_CONFIG_HOME/zed` falling back to `~/.zed` if that environment variable is not set.

```
$ zed config set-context rbac rbac_example jimmy@authzed.com
NAME	TENANT      	TOKEN NAME       	ENDPOINT            	CURRENT
rbac	rbac_example	jimmy@authzed.com	grpc.authzed.com:443

$ zed config use-context rbac
NAME	TENANT      	TOKEN NAME       	ENDPOINT            	CURRENT
rbac	rbac_example	jimmy@authzed.com	grpc.authzed.com:443	true
```

The environment variables `$ZED_TENANT`, `$ZED_TOKEN`, and `$ZED_ENDPOINT` can be used to override their respective values in the current context.

### Explore relationships

The `describe` command provides a tree view of a namespace definition.

```
$ zed describe document
document
 ├── writer
 └── reader
      └── union
           ├── _this
           └── TUPLE_OBJECT: writer
```

The `expand` command provides a tree view of a relation of a particular object.

```
$ zed expand document:firstdoc reader
document:firstdoc reader
 └── union
      ├── user:fred
      └── document:firstdoc writer
           └── user:tom
```

When piped or provided the `--json` flag, API responses are converted into JSON.

```
$ zed describe document | jq '.config.relation[0].name'
"writer"
```

### Modify relationships

```
$ zed check user:jimmy document:firstdoc reader
false

$ zed create user:jimmy document:firstdoc writer
CAESAwiLBA==

$ zed check user:jimmy document:firstdoc writer
true

$ zed check user:jimmy document:firstdoc reader
true

$ zed delete user:jimmy document:firstdoc writer
CAESAwiMBA==

$ zed check user:jimmy document:firstdoc reader
false

$ zed check user:jimmy document:firstdoc writer
false
```
