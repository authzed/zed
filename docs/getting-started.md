import { Callout } from 'nextra/components'

# Installing Zed

[Zed](https://github.com/authzed/zed) is the CLI used to interact with SpiceDB.

It is built as a standalone executable file which simplifies installation, but one should prefer one of the recommended installation methods detailed below.

## Debian packages

[Debian-based Linux] users can install SpiceDB packages by adding an additional apt source.

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

```
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

[Debian-based Linux]: https://en.wikipedia.org/wiki/List_of_Linux_distributions#Debian-based

## RPM packages

[RPM-based Linux] users can install packages by adding a new yum repository:

```sh
sudo cat << EOF >> /etc/yum.repos.d/Authzed-Fury.repo
[authzed-fury]
name=AuthZed Fury Repository
baseurl=https://pkg.authzed.com/yum/
enabled=1
gpgcheck=0
EOF
```

Install as usual:

```sh
sudo dnf install -y zed
```

[RPM-based Linux]: https://en.wikipedia.org/wiki/List_of_Linux_distributions#RPM-based

## Homebrew (macOS)

macOS users can install packages by adding a [Homebrew tap]:

```sh
brew install authzed/tap/zed
```

[Homebrew tap]: https://docs.brew.sh/Taps

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

Afterwards, you can run it with `docker run`:

```sh
docker run --rm authzed/zed version
```

#### Downloading the binary

Visit the GitHub release page for the [latest release](https://github.com/authzed/zed/releases/latest).
Scroll down to the `Assets` section and download the appropriate artifact.

#### Source

Clone the GitHub repository:

```sh
git clone git@github.com:authzed/zed.git
```

Enter the directory and build the binary using mage:

```sh
cd zed
go build ./cmd/zed
```

You can find more commands for tasks such as testing, linting in the repository's [CONTRIBUTING.md].

[CONTRIBUTING.md]: https://github.com/authzed/zed/blob/main/CONTRIBUTING.md

