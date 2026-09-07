# Deployments

Deploy a Docker image to a VM onctl already manages: `deploy` pulls the image
**locally**, saves it with gzip compression, uploads it over SSH, and runs it on
the remote host.

```bash
Usage:
  onctl deploy VM_NAME

Flags:
  -e, --env strings    Environment variables for the container
  -h, --help           help for deploy
  -i, --image string   Docker image to deploy (required)
  -n, --name string    Name for the Docker container
```

:::warning Matching architectures
The image is pulled on **your machine**, not the remote VM -- make sure its
architecture matches the VM's (e.g. don't push an `arm64` image to an `amd64`
VM). A mismatch fails when the container starts on the remote, not at upload
time, with an `exec format error`.
:::

## Prerequisites

- **Docker installed locally** -- `deploy` runs `docker pull`/`docker save` on
  your own machine to prepare the image.
- **Docker installed on the target VM** -- `deploy` loads and runs the image
  there, it doesn't install Docker for you. See [Templates](./templates) for
  bootstrapping a VM with Docker via `-a docker/docker.sh` at create time.

## Private registries

Since the image is pulled locally, private-registry access (e.g. `docker
login`) only needs to be set up wherever you run `onctl deploy` from -- the
remote VM never talks to the registry directly.

## Example

```bash
onctl deploy my-box -i nginx:alpine -n web -e FOO=bar
```
