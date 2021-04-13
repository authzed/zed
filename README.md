# zed

A client for managing [authzed] or any API-compatible system from your command line.

[authzed]: https://authzed.com

## Getting Started

Configuring credentials is similar to [kubeconfig] in [kubectl].
API Tokens are stored in the system keychain and context data is stored in `$XDG_CONFIG_HOME/zed` falling back to `~/.zed` if that environment variable is not set.
If both `$ZED_TENANT` and `$ZED_TOKEN` are set, these values are used instead of the current context.

[kubeconfig]: https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/
[kubectl]: https://kubernetes.io/docs/reference/kubectl/overview/

```sh 
$ zed config set-token default-token my_default_token_d3adb33f
NAME        	TOKEN
default-test	<redacted>

$ zed config set-context default mytenant default-token
NAME    	TENANT  	TOKEN NAME  	CURRENT
default 	mytenant	default-test

$ zed config use-context default
NAME    	TENANT  	TOKEN NAME  	CURRENT
default 	mytenant	default-test	true
```

Viewing a namespace

```sh
$ zed describe document
readme_example/document
 ├── writer
 └── reader
      └── union
           ├── _this
           └── TUPLE_OBJECT: writer
```

Parsing a namespace with JSON tooling

```sh
$ zed describe document | jq '.config.relation[0].name'
"writer"
```
