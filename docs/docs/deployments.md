# Deployments

The main component to deploy your app with ease. 

```bash 
Usage:
  onctl deploy [flags]

Flags:
      --cnames strings   CNAMES link to this app
  -c, --cpu string       CPU (m) limit of the app. (default "250")
      --env string       Name of environment variable group
  -h, --help             help for deploy
  -i, --image string     ImageName and Tag ex. nginx:latest
  -m, --memory string    Memory (Mi) limit of the app. (default "250")
      --name string      Name of the app.
  -p, --port int32       Port of the app. (default 80)
      --public           makes deployment public and accessible from a onkube.app subdomain
  -v, --volume string    Volume <name>:<mount_path> to mount.

```

:::warning Public Deployments
    Deployment are by default **not** exposed to internet. In order to get a public URL
    You should use --public option
:::

## Image

The url of the image to deploy. ex. `alpine:latest` / `nginx:alpine` etc.

:::note To Deploy an image from a private repository
    You should add your access credentials first

    * `onctl reg add <image_url> -u <user> -p <password>` - Add your container registry credentials 
:::

## Environment Variables

1. Define your Environment Variables.
2. Pass environment variable group name to deploy command 
```bash
    onctl deploy -i nginx:alpine --env <name>
```
