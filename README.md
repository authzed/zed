# zed

A client for managing [authzed] from your command line

[authzed]: https://authzed.com

## Getting Started

```sh 
# Configure credentials for the service, similar to kubectl:
zed config set-token default-token my_default_token_d3adb33f
zed config set-context default mytenant default-token
zed config use-context default

# Describe a namespace
zed describe document
readme_example/document
 ├── writer
 └── reader
      └── union
           ├── _this
           └── TUPLE_OBJECT: writer
```
