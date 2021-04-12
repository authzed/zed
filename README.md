# zed

A client for managing [authzed] from your command line

[authzed]: https://authzed.com

## Getting Started

Configuring credentials for the service, similar to kubectl:

```sh 
$ zed config set-token default-token my_default_token_d3adb33f
$ zed config set-context default mytenant default-token
$ zed config use-context default
$ zed config list-contexts
default: mytenant via default-token (current)
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
