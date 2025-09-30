## Reference: `zed`

A command-line client for managing SpiceDB clusters.

### Examples

```

zed context list
zed context set dev localhost:80 testpresharedkey --insecure
zed context set prod grpc.authzed.com:443 tc_zed_my_laptop_deadbeefdeadbeefdeadbeefdeadbeef
zed context use dev
zed permission check --explain document:firstdoc writer user:emilia

```

### Options

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```

### Children commands

- [zed backup](#reference-zed-backup)	 - Create, restore, and inspect permissions system backups
- [zed context](#reference-zed-context)	 - Manage configurations for connecting to SpiceDB deployments
- [zed import](#reference-zed-import)	 - Imports schema and relationships from a file or url
- [zed mcp](#reference-zed-mcp)	 - MCP (Model Context Protocol) server commands
- [zed permission](#reference-zed-permission)	 - Query the permissions in a permissions system
- [zed relationship](#reference-zed-relationship)	 - Query and mutate the relationships in a permissions system
- [zed schema](#reference-zed-schema)	 - Manage schema for a permissions system
- [zed use](#reference-zed-use)	 - Alias for `zed context use`
- [zed validate](#reference-zed-validate)	 - Validates the given validation file (.yaml, .zaml) or schema file (.zed)
- [zed version](#reference-zed-version)	 - Display zed and SpiceDB version information


## Reference: `zed backup`

Create, restore, and inspect permissions system backups

```
zed backup <filename> [flags]
```

### Options

```
      --page-limit uint32      defines the number of relationships to be read by requested page during backup
      --prefix-filter string   include only schema and relationships with a given prefix
      --rewrite-legacy         potentially modify the schema to exclude legacy/broken syntax
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```

### Children commands

- [zed backup create](#reference-zed-backup-create)	 - Backup a permission system to a file
- [zed backup parse-relationships](#reference-zed-backup-parse-relationships)	 - Extract the relationships from a backup file
- [zed backup parse-revision](#reference-zed-backup-parse-revision)	 - Extract the revision from a backup file
- [zed backup parse-schema](#reference-zed-backup-parse-schema)	 - Extract the schema from a backup file
- [zed backup redact](#reference-zed-backup-redact)	 - Redact a backup file to remove sensitive information
- [zed backup restore](#reference-zed-backup-restore)	 - Restore a permission system from a file


## Reference: `zed backup create`

Backup a permission system to a file

```
zed backup create <filename> [flags]
```

### Options

```
      --page-limit uint32      defines the number of relationships to be read by requested page during backup
      --prefix-filter string   include only schema and relationships with a given prefix
      --rewrite-legacy         potentially modify the schema to exclude legacy/broken syntax
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed backup parse-relationships`

Extract the relationships from a backup file

```
zed backup parse-relationships <filename> [flags]
```

### Options

```
      --prefix-filter string   Include only relationships with a given prefix
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed backup parse-revision`

Extract the revision from a backup file

```
zed backup parse-revision <filename>
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed backup parse-schema`

Extract the schema from a backup file

```
zed backup parse-schema <filename> [flags]
```

### Options

```
      --prefix-filter string   include only schema and relationships with a given prefix
      --rewrite-legacy         potentially modify the schema to exclude legacy/broken syntax
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed backup redact`

Redact a backup file to remove sensitive information

```
zed backup redact <filename> [flags]
```

### Options

```
      --print-redacted-object-ids   prints the redacted object IDs
      --redact-definitions          redact definitions (default true)
      --redact-object-ids           redact object IDs (default true)
      --redact-relations            redact relations (default true)
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed backup restore`

Restore a permission system from a file

```
zed backup restore <filename> [flags]
```

### Options

```
      --batch-size uint                restore relationship write batch size (default 1000)
      --batches-per-transaction uint   number of batches per transaction (default 10)
      --conflict-strategy string       strategy used when a conflicting relationship is found. Possible values: fail, skip, touch (default "fail")
      --disable-retries                retries when an errors is determined to be retryable (e.g. serialization errors)
      --prefix-filter string           include only schema and relationships with a given prefix
      --request-timeout duration       timeout for each request performed during restore (default 30s)
      --rewrite-legacy                 potentially modify the schema to exclude legacy/broken syntax
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed context`

Manage configurations for connecting to SpiceDB deployments

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```

### Children commands

- [zed context list](#reference-zed-context-list)	 - Lists all available contexts
- [zed context remove](#reference-zed-context-remove)	 - Removes a context
- [zed context set](#reference-zed-context-set)	 - Creates or overwrite a context
- [zed context use](#reference-zed-context-use)	 - Sets a context as the current context


## Reference: `zed context list`

Lists all available contexts

```
zed context list [flags]
```

### Options

```
      --reveal-tokens   display secrets in results
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed context remove`

Removes a context

```
zed context remove <system>
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed context set`

Creates or overwrite a context

```
zed context set <name> <endpoint> <api-token>
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed context use`

Sets a context as the current context

```
zed context use <system>
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed import`

Imports schema and relationships from a file or url

```
zed import <url> [flags]
```

### Examples

```

	From a gist:
		zed import https://gist.github.com/ecordell/8e3b613a677e3c844742cf24421c08b6

	From a playground link:
		zed import https://play.authzed.com/s/iksdFvCtvnkR/schema

	From pastebin:
		zed import https://pastebin.com/8qU45rVK

	From a devtools instance:
		zed import https://localhost:8443/download

	From a local file (with prefix):
		zed import file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

	From a local file (no prefix):
		zed import authzed-x7izWU8_2Gw3.yaml

	Only schema:
		zed import --relationships=false file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

	Only relationships:
		zed import --schema=false file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

	With schema definition prefix:
		zed import --schema-definition-prefix=mypermsystem file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

```

### Options

```
      --batch-size int                    import batch size (default 1000)
      --relationships                     import relationships (default true)
      --schema                            import schema (default true)
      --schema-definition-prefix string   prefix to add to the schema's definition(s) before importing
      --workers int                       number of concurrent batching workers (default 1)
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed mcp`

MCP (Model Context Protocol) server commands.
	
The MCP server provides tooling and resources for developing and debugging SpiceDB schema and relationships. The server runs an in-memory development instance of SpiceDB and does not connect to a running instance of SpiceDB.

To use with Claude Code, run "zed mcp experimental-run" to start the SpiceDB Dev MCP server and then run "claude mcp add --transport http spicedb http://localhost:9999/mcp" to add the server to your Claude Code integrations.


### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```

### Children commands

- [zed mcp experimental-run](#reference-zed-mcp-experimental-run)	 - Run the Experimental MCP server


## Reference: `zed mcp experimental-run`

Run the Experimental MCP server

```
zed mcp experimental-run [flags]
```

### Options

```
  -p, --port int   port for the HTTP streaming server (default 9999)
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed permission`

Query the permissions in a permissions system

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```

### Children commands

- [zed permission bulk](#reference-zed-permission-bulk)	 - Check a permissions in bulk exists for a resource-subject pairs
- [zed permission check](#reference-zed-permission-check)	 - Check that a permission exists for a subject
- [zed permission expand](#reference-zed-permission-expand)	 - Expand the structure of a permission
- [zed permission lookup-resources](#reference-zed-permission-lookup-resources)	 - Enumerates resources of a given type for which the subject has permission
- [zed permission lookup-subjects](#reference-zed-permission-lookup-subjects)	 - Enumerates the subjects of a given type for which the subject has permission on the resource


## Reference: `zed permission bulk`

Check a permissions in bulk exists for a resource-subject pairs

```
zed permission bulk <resource:id#permission@subject:id> <resource:id#permission@subject:id> ... [flags]
```

### Options

```
      --consistency-at-exactly string   evaluate at the provided zedtoken
      --consistency-at-least string     evaluate at least as consistent as the provided zedtoken
      --consistency-full                evaluate at the newest zedtoken in the database
      --consistency-min-latency         evaluate at the zedtoken preferred by the database
      --explain                         requests debug information from SpiceDB and prints out a trace of the requests
      --json                            output as JSON
      --revision string                 optional revision at which to check
      --schema                          requests debug information from SpiceDB and prints out the schema used
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed permission check`

Check that a permission exists for a subject

```
zed permission check <resource:id> <permission> <subject:id> [flags]
```

### Options

```
      --caveat-context string           the caveat context to send along with the check, in JSON form
      --consistency-at-exactly string   evaluate at the provided zedtoken
      --consistency-at-least string     evaluate at least as consistent as the provided zedtoken
      --consistency-full                evaluate at the newest zedtoken in the database
      --consistency-min-latency         evaluate at the zedtoken preferred by the database
      --error-on-no-permission          if true, zed will return exit code 1 if subject does not have unconditional permission
      --explain                         requests debug information from SpiceDB and prints out a trace of the requests
      --json                            output as JSON
      --schema                          requests debug information from SpiceDB and prints out the schema used
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed permission expand`

Expand the structure of a permission

```
zed permission expand <permission> <resource:id> [flags]
```

### Options

```
      --consistency-at-exactly string   evaluate at the provided zedtoken
      --consistency-at-least string     evaluate at least as consistent as the provided zedtoken
      --consistency-full                evaluate at the newest zedtoken in the database
      --consistency-min-latency         evaluate at the zedtoken preferred by the database
      --json                            output as JSON
      --revision string                 optional revision at which to check
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed permission lookup-resources`

Enumerates resources of a given type for which the subject has permission

```
zed permission lookup-resources <type> <permission> <subject:id> [flags]
```

### Options

```
      --caveat-context string           the caveat context to send along with the lookup, in JSON form
      --consistency-at-exactly string   evaluate at the provided zedtoken
      --consistency-at-least string     evaluate at least as consistent as the provided zedtoken
      --consistency-full                evaluate at the newest zedtoken in the database
      --consistency-min-latency         evaluate at the zedtoken preferred by the database
      --cursor string                   resume pagination from a specific cursor token
      --json                            output as JSON
      --page-limit uint32               limit of relations returned per page
      --revision string                 optional revision at which to check
      --show-cursor                     display the cursor token after pagination (default true)
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed permission lookup-subjects`

Enumerates the subjects of a given type for which the subject has permission on the resource

```
zed permission lookup-subjects <resource:id> <permission> <subject_type#optional_subject_relation> [flags]
```

### Options

```
      --caveat-context string           the caveat context to send along with the lookup, in JSON form
      --consistency-at-exactly string   evaluate at the provided zedtoken
      --consistency-at-least string     evaluate at least as consistent as the provided zedtoken
      --consistency-full                evaluate at the newest zedtoken in the database
      --consistency-min-latency         evaluate at the zedtoken preferred by the database
      --json                            output as JSON
      --revision string                 optional revision at which to check
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed relationship`

Query and mutate the relationships in a permissions system

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```

### Children commands

- [zed relationship bulk-delete](#reference-zed-relationship-bulk-delete)	 - Deletes relationships matching the provided pattern en masse
- [zed relationship create](#reference-zed-relationship-create)	 - Create a relationship for a subject
- [zed relationship delete](#reference-zed-relationship-delete)	 - Deletes a relationship
- [zed relationship read](#reference-zed-relationship-read)	 - Enumerates relationships matching the provided pattern
- [zed relationship touch](#reference-zed-relationship-touch)	 - Idempotently updates a relationship for a subject
- [zed relationship watch](#reference-zed-relationship-watch)	 - Watches the stream of relationship updates and schema updates from the server


## Reference: `zed relationship bulk-delete`

Deletes relationships matching the provided pattern en masse

```
zed relationship bulk-delete <resource_type:optional_resource_id> <optional_relation> <optional_subject_type:optional_subject_id#optional_subject_relation> [flags]
```

### Options

```
      --force                   force deletion of all elements in batches defined by <optional-limit>
      --optional-limit uint32   the max amount of elements to delete. If you want to delete all in batches of size <optional-limit>, set --force to true (default 1000)
      --subject-filter string   optional subject filter
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed relationship create`

Create a relationship for a subject

```
zed relationship create <resource:id> <relation> <subject:id#optional_subject_relation> [flags]
```

### Options

```
  -b, --batch-size int           batch size when writing streams of relationships from stdin (default 100)
      --caveat string            the caveat for the relationship, with format: 'caveat_name:{"some":"context"}'
      --expiration-time string   the expiration time of the relationship in RFC 3339 format
      --json                     output as JSON
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed relationship delete`

Deletes a relationship

```
zed relationship delete <resource:id> <relation> <subject:id#optional_subject_relation> [flags]
```

### Options

```
  -b, --batch-size int   batch size when deleting streams of relationships from stdin (default 100)
      --json             output as JSON
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed relationship read`

Enumerates relationships matching the provided pattern.

To filter returned relationships using a resource ID prefix, append a '%' to the resource ID:

zed relationship read some-type:some-prefix-%


```
zed relationship read <resource_type:optional_resource_id> <optional_relation> <optional_subject_type:optional_subject_id#optional_subject_relation> [flags]
```

### Options

```
      --consistency-at-exactly string   evaluate at the provided zedtoken
      --consistency-at-least string     evaluate at least as consistent as the provided zedtoken
      --consistency-full                evaluate at the newest zedtoken in the database
      --consistency-min-latency         evaluate at the zedtoken preferred by the database
      --json                            output as JSON
      --page-limit uint32               limit of relations returned per page (default 100)
      --subject-filter string           optional subject filter
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed relationship touch`

Idempotently updates a relationship for a subject

```
zed relationship touch <resource:id> <relation> <subject:id#optional_subject_relation> [flags]
```

### Options

```
  -b, --batch-size int           batch size when writing streams of relationships from stdin (default 100)
      --caveat string            the caveat for the relationship, with format: 'caveat_name:{"some":"context"}'
      --expiration-time string   the expiration time for the relationship in RFC 3339 format
      --json                     output as JSON
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed relationship watch`

Watches the stream of relationship updates and schema updates from the server

```
zed relationship watch [object_types, ...] [start_cursor] [flags]
```

### Options

```
      --filter optional_resource_type:optional_resource_id_or_prefix#optional_relation@optional_subject_filter   optional filter(s) for the watch stream. Example: optional_resource_type:optional_resource_id_or_prefix#optional_relation@optional_subject_filter
      --object_types strings                                                                                     optional object types to watch updates for
      --revision string                                                                                          optional revision at which to start watching
      --timestamp                                                                                                shows timestamp of incoming update events
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed schema`

Manage schema for a permissions system

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```

### Children commands

- [zed schema compile](#reference-zed-schema-compile)	 - Compile a schema that uses extended syntax into one that can be written to SpiceDB
- [zed schema copy](#reference-zed-schema-copy)	 - Copy a schema from one context into another
- [zed schema diff](#reference-zed-schema-diff)	 - Diff two schema files
- [zed schema read](#reference-zed-schema-read)	 - Read the schema of a permissions system
- [zed schema write](#reference-zed-schema-write)	 - Write a schema file (.zed or stdin) to the current permissions system


## Reference: `zed schema compile`

Compile a schema that uses extended syntax into one that can be written to SpiceDB

```
zed schema compile <file> [flags]
```

### Examples

```

	Write to stdout:
		zed preview schema compile root.zed
	Write to an output file:
		zed preview schema compile root.zed --out compiled.zed
	
```

### Options

```
      --out string   output filepath; omitting writes to stdout
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed schema copy`

Copy a schema from one context into another

```
zed schema copy <src context> <dest context> [flags]
```

### Options

```
      --json                              output as JSON
      --schema-definition-prefix string   prefix to add to the schema's definition(s) before writing
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed schema diff`

Diff two schema files

```
zed schema diff <before file> <after file>
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed schema read`

Read the schema of a permissions system

```
zed schema read [flags]
```

### Options

```
      --json   output as JSON
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed schema write`

Write a schema file (.zed or stdin) to the current permissions system

```
zed schema write <file?> [flags]
```

### Options

```
      --json                              output as JSON
      --schema-definition-prefix string   prefix to add to the schema's definition(s) before writing
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed use`

Alias for `zed context use`

```
zed use <context>
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed validate`

Validates the given validation file (.yaml, .zaml) or schema file (.zed)

```
zed validate <validation_file_or_schema_file> [flags]
```

### Examples

```

	From a local file (with prefix):
		zed validate file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

	From a local file (no prefix):
		zed validate authzed-x7izWU8_2Gw3.yaml

	From a gist:
		zed validate https://gist.github.com/ecordell/8e3b613a677e3c844742cf24421c08b6

	From a playground link:
		zed validate https://play.authzed.com/s/iksdFvCtvnkR/schema

	From pastebin:
		zed validate https://pastebin.com/8qU45rVK

	From a devtools instance:
		zed validate https://localhost:8443/download
```

### Options

```
      --fail-on-warn         treat warnings as errors during validation
      --force-color          force color code output even in non-tty environments
      --schema-type string   force validation according to specific schema syntax ("", "composable", "standard")
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



## Reference: `zed version`

Display zed and SpiceDB version information

```
zed version [flags]
```

### Options

```
      --include-deps             include dependencies' versions
      --include-remote-version   whether to display the version of Authzed or SpiceDB for the current context (default true)
```

### Options Inherited From Parent Flags

```
      --certificate-path string     path to certificate authority used to verify secure connections
      --endpoint string             spicedb gRPC API endpoint
      --hostname-override string    override the hostname used in the connection to the endpoint
      --insecure                    connect over a plaintext connection
      --log-format string           format of logs ("auto", "console", "json") (default "auto")
      --log-level string            verbosity of logging ("trace", "debug", "info", "warn", "error") (default "info")
      --max-message-size int        maximum size *in bytes* (defaults to 4_194_304 bytes ~= 4MB) of a gRPC message that can be sent or received by zed
      --max-retries uint            maximum number of sequential retries to attempt when a request fails (default 10)
      --no-verify-ca                do not attempt to verify the server's certificate chain and host name
      --permissions-system string   permissions system to query
      --proxy string                specify a SOCKS5 proxy address
      --request-id string           optional id to send along with SpiceDB requests for tracing
      --skip-version-check          if true, no version check is performed against the server
      --token string                token used to authenticate to SpiceDB
```



