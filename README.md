# zed

A client for managing [authzed] or any API-compatible system from your command line.

[authzed]: https://authzed.com

## Getting Started

Configuring credentials is similar to [kubeconfig] in [kubectl].
API Tokens are stored in the system keychain and context data is stored in `$XDG_CONFIG_HOME/zed` falling back to `~/.zed` if that environment variable is not set.
If both `$ZED_TENANT` and `$ZED_TOKEN` are set, these values are used instead of the current context.

[kubeconfig]: https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/
[kubectl]: https://kubernetes.io/docs/reference/kubectl/overview/

Managing tokens:

```sh
$ zed config set-token jimmy@authzed.com tu_zed_hanazawa_deadbeefdeadbeefdeadbeefdeadbeef
NAME             	ENDPOINT            	TOKEN     	MODIFIED
jimmy@authzed.com	grpc.authzed.com:443	<redacted>	now

$ zed config get-tokens
NAME             	ENDPOINT            	TOKEN     	MODIFIED
jimmy@authzed.com	grpc.authzed.com:443	<redacted>	2 minutes ago
```

Managing contexts:

```sh
$ zed config set-context rbac rbac_example jimmy@authzed.com
NAME	TENANT      	TOKEN NAME       	ENDPOINT            	CURRENT
rbac	rbac_example	jimmy@authzed.com	grpc.authzed.com:443

$ zed config use-context rbac
NAME	TENANT      	TOKEN NAME       	ENDPOINT            	CURRENT
rbac	rbac_example	jimmy@authzed.com	grpc.authzed.com:443	true
```

Viewing a namespace

```sh
$ zed describe document
rbac_example/document
 ├── writer
 └── reader
      └── union
           ├── _this
           └── TUPLE_OBJECT: writer
```

Checking, creating, deleting a relation:

```sh
$ zed check user:tom document:firstdoc writer
true

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

Piping into JSON tooling:

```sh
$ zed describe document | jq '.config.relation[0].name'
"writer"

$ zed check user:tom document:firstdoc writer | jq
{
  "isMember": true,
  "revision": {
    "token": "CAESAwiKBA=="
  }
}
```
