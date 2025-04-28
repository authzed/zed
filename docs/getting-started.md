# zed

## Getting Started

### Installing the binary

Binary releases are available for Linux, macOS, and Windows on AMD64 and ARM64 architectures.

[Homebrew] users for both macOS and Linux can install the latest binary releases of zed using the official tap:

```sh
brew install authzed/tap/zed
```

[Debian-based Linux] users can install zed packages by adding a new APT source:

First, download the public signing key for the repository:

```sh
# In releases older than Debian 12 and Ubuntu 22.04, the folder `/etc/apt/keyrings` does not exist by default, and it should be created before the curl command.
# sudo mkdir -p -m 755 /etc/apt/keyrings

curl -sS https://pkg.authzed.com/apt/gpg.key | sudo gpg --dearmor --yes -o /etc/apt/keyrings/authzed.gpg
```

Then add the list file for the repository:

```sh
echo "deb [signed-by=/etc/apt/keyrings/authzed.gpg] https://pkg.authzed.com/apt/ * *"  | sudo tee /etc/apt/sources.list.d/authzed.list
sudo chmod 644 /etc/apt/sources.list.d/authzed.list  # helps tools such as command-not-found to work correctly

```

Alternatively, if you want to use the new `deb822`-style `authzed.sources` format, put the following in `/etc/apt/sources.list.d/authzed.sources`:

```yaml
Types: deb
URIs: https://pkg.authzed.com/apt/
Suites: *
Components: *
Signed-By: /etc/apt/keyrings/authzed.gpg
```

Once you've defined the sources and updated your apt cache, it can be installed just like any other package:

```sh
sudo apt update
sudo apt install -y zed
```

[RPM-based Linux] users can install zed packages by adding a new YUM repository:

```sh
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

### Other methods

#### Docker

Container images are available for AMD64 and ARM64 architectures on the following registries:

- [authzed/zed](https://hub.docker.com/r/authzed/zed)
- [ghcr.io/authzed/zed](https://github.com/authzed/zed/pkgs/container/zed)
- [quay.io/authzed/zed](https://quay.io/authzed/zed)

You can pull down the latest stable release:

```sh
docker pull authzed/zed
```

Afterward, you can run it with `docker run`:

```sh
docker run --rm authzed/zed version
```
