# zed

[![Docs](https://img.shields.io/badge/docs-authzed.com-%234B4B6C "Authzed Documentation")](https://authzed.com/docs)
[![YouTube](https://img.shields.io/youtube/channel/views/UCFeSgZf0rPqQteiTQNGgTPg?color=%23F40203&logo=youtube&style=flat-square&label=YouTube "Authzed YouTube Channel")](https://www.youtube.com/channel/UCFeSgZf0rPqQteiTQNGgTPg)
[![Discord Server](https://img.shields.io/discord/844600078504951838?color=7289da&logo=discord "Discord Server")](https://authzed.com/discord)
[![Twitter](https://img.shields.io/badge/twitter-%40authzed-1D8EEE?logo=twitter "@authzed on Twitter")](https://twitter.com/authzed)

A command-line client for managing [SpiceDB].

[SpiceDB]: https://github.com/authzed/spicedb

zed features include:

- Context switching that stores credentials securely in your OS keychain
- Check, Expand, Lookup Resources, Lookup Subjects commands for Permissions
- Create, Read, Touch, Delete, Bulk-Delete commands for Relationships
- Read, Write, Validate, Import, Copy commands for Schemas
- Experimental Backup and Restore commands

Have questions? Ask in our [Discord].

Looking to contribute? See [CONTRIBUTING.md].

You can find issues by priority: [Urgent], [High], [Medium], [Low], [Maybe].
There are also [good first issues].

[Discord]: https://authzed.com/discord
[CONTRIBUTING.md]: https://github.com/authzed/spicedb/blob/main/CONTRIBUTING.md
[Urgent]: https://github.com/authzed/spicedb/labels/priority%2F0%20urgent
[High]: https://github.com/authzed/spicedb/labels/priority%2F1%20high
[Medium]: https://github.com/authzed/spicedb/labels/priority%2F2%20medium
[Low]: https://github.com/authzed/spicedb/labels/priority%2F3%20low
[Maybe]: https://github.com/authzed/spicedb/labels/priority%2F4%20maybe
[good first issues]: https://github.com/authzed/spicedb/labels/hint%2Fgood%20first%20issue

## Getting Started

### Installing the binary

Binary releases are available for Linux, macOS, and Windows on AMD64 and ARM64 architectures.

[Homebrew] users for both macOS and Linux can install the latest binary releases of zed using the official tap:

```command
brew install authzed/tap/zed
```

[Debian-based Linux] users can install zed packages by adding a new APT source:

```command
sudo apt update && sudo apt install -y curl ca-certificates gpg
curl https://apt.fury.io/authzed/gpg.key | sudo apt-key add -
sudo sh -c 'echo "deb https://apt.fury.io/authzed/ * *" > /etc/apt/sources.list.d/fury.list'
sudo apt update && sudo apt install -y zed
```

[RPM-based Linux] users can install zed packages by adding a new YUM repository:

```command
sudo cat << EOF >> /etc/yum.repos.d/Authzed-Fury.repo
[authzed-fury]
name=AuthZed Fury Repository
baseurl=https://yum.fury.io/authzed/
enabled=1
gpgcheck=0
EOF
sudo dnf install -y zed
```

[homebrew]: https://docs.authzed.com/spicedb/installing#brew
[Debian-based Linux]: https://en.wikipedia.org/wiki/List_of_Linux_distributions#Debian-based
[RPM-based Linux]: https://en.wikipedia.org/wiki/List_of_Linux_distributions#RPM-based

### Creating a context

Contexts store connection credentials for accessing SpiceDB clusters securely in the OS keychain.
Before performing most commands, a context must be set.
Alternatively, you can provide context values via environment variables which will override the existing context for that execution.

The `zed context` subcommand has operations for setting the current, creating, listing, deleting contexts:

```sh
zed context set prod grpc.authzed.com:443 tc_zed_my_laptop_deadbeefdeadbeefdeadbeefdeadbeef
zed context set dev localhost:80 testpresharedkey
zed context list
```

### Headless usage

If you provide all context values (e.g. `ZED_ENDPOINT`, `ZED_TOKEN`) as environment variables or flags (e.g. `--endpoint`, `--token`), you do not need to set a context.
You can also provide the `ZED_KEYRING_PASSWORD` environment variable to access an existing context in a non-interactive way.

```sh
zed schema read --endpoint grpc.authzed.com:443 --token tc_zed_my_laptop_deadbeefdeadbeef
ZED_ENDPOINT=grpc.authzed.com:443 ZED_TOKEN=tc_zed_my_laptop_deadbeefdeadbeef zed schema read
ZED_KEYRING_PASSWORD=redacted zed schema read
```

### Debugging

The `--explain` flag can be used on `permission check` to see a trace:

```sh
zed permission check document:firstdoc writer user:emilia --explain
```

## Acknowledgements

zed is a community project fueled by contributions from both organizations and individuals.
We appreciate all contributions, large and small, and would like to thank all those involved.

In addition, we'd like to highlight a few notable contributions:

- The GitHub Authorization Team for implementing the bulk-delete command
