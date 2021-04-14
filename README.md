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

**Note:** The master branch may be in an unstable or even broken state during development.
Please use [releases] instead of the master branch in order to get stable binaries.

A client for managing [authzed] or any API-compatible system from your command line.

[authzed]: https://authzed.com

## Example Usage

### Managing credentials

Configuring credentials is similar to [kubeconfig] in [kubectl].
If both `$ZED_TENANT` and `$ZED_TOKEN` are set, these values are used instead of the current context.

[kubeconfig]: https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/
[kubectl]: https://kubernetes.io/docs/reference/kubectl/overview/

API Tokens are stored in the system keychain.

```sh
$ zed config set-token jimmy@authzed.com tu_zed_hanazawa_deadbeefdeadbeefdeadbeefdeadbeef
NAME             	ENDPOINT            	TOKEN     	MODIFIED
jimmy@authzed.com	grpc.authzed.com:443	<redacted>	now

$ zed config get-tokens
NAME             	ENDPOINT            	TOKEN     	MODIFIED
jimmy@authzed.com	grpc.authzed.com:443	<redacted>	2 minutes ago
```

Context data is stored in `$XDG_CONFIG_HOME/zed` falling back to `~/.zed` if that environment variable is not set.

```sh
$ zed config set-context rbac rbac_example jimmy@authzed.com
NAME	TENANT      	TOKEN NAME       	ENDPOINT            	CURRENT
rbac	rbac_example	jimmy@authzed.com	grpc.authzed.com:443

$ zed config use-context rbac
NAME	TENANT      	TOKEN NAME       	ENDPOINT            	CURRENT
rbac	rbac_example	jimmy@authzed.com	grpc.authzed.com:443	true
```

### Managing namespaces

```sh
$ zed describe document
rbac_example/document
 ├── writer
 └── reader
      └── union
           ├── _this
           └── TUPLE_OBJECT: writer

```

When piped or provided the `--json` flag, API responses are converted into JSON.

```
$ zed describe document | jq '.config.relation[0].name'
"writer"
```

### Managing relationships

```sh
$ zed check user:tom document:firstdoc writer
true

$ zed check user:tom document:firstdoc writer | jq
{
  "isMember": true,
  "revision": {
    "token": "CAESAwiKBA=="
  }
}

$ zed check user:jimmy document:firstdoc writer
false

$ zed create user:jimmy document:firstdoc writer
CAESAwiLBA==

$ zed check user:jimmy document:firstdoc writer
true

$ zed delete user:jimmy document:firstdoc writer
CAESAwiMBA==

$ zed check user:jimmy document:firstdoc writer
false
```
