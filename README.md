## onctl - Preview Environments for multi-cloud

[![Build Status](https://github.com/cdalar/onctl/actions/workflows/build.yml/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/build.yml)

*onctl* was created to dynamically create preview environment based on docker-compose on a single vm. 

1. Starts a vm on defined cloud (supports aws, hetzer at the moment)
2. Installs docker package and make necessary adjustments.

### Example repo. 

Please check [onctl-demo](https://github.com/cdalar/onctl-demo) repo for how to use this tool inside github-actions

### Use it on your local machine directly

```
cd <into your git folder>
onctl create 
```
This should start the vm and make it ready to use. Then;
Run these to deploy your docker compose app

```
ssh-keyscan $(cat onctl-deploy.json | jq -r '.public_ip') >> ~/.ssh/known_hosts
DOCKER_HOST=$(cat onctl-deploy.json | jq -r '.docker_host') docker compose up -d --build
```

Have fun!

