## onctl - Preview Environments for multi-cloud

[![Build Status](https://github.com/cdalar/onctl/actions/workflows/build.yml/badge.svg)](https://github.com/cdalar/onctl/actions/workflows/build.yml)

*onctl* was created to dynamically create preview environment based on docker-compose on a single vm. 

1. Starts a vm on defined cloud (supports aws, hetzer at the moment)
2. Installs docker package and make necessary adjustments.

## Getting Started 

For MacOS (ARM or Intel) 

```
brew install cdalar/tap/onctl
```

For Linux (amd64)

```
wget https://www.github.com/cdalar/onctl/releases/latest/download/onctl-linux-amd64.tar.gz
tar zxvf onctl-linux-amd64.tar.gz
sudo mv onctl /usr/local/bin/
```

## Github Action
You can use this action to integrate onctl on your pipeline [onctl-action](https://github.com/marketplace/actions/onctl-action). You on every PR you created you can have an ready to check environment.


## Example/Template Repository

Please check [onctl-demo](https://github.com/cdalar/onctl-demo) repo for how to use this tool inside github-actions

## Use it on your local machine directly

```
cd <into your git folder>
onctl create 
```
This should start the vm and make it ready to use. Then;
Run these to deploy your docker compose app

```
ssh-keyscan $(jq -r .public_ip onctl-deploy.json) >> ~/.ssh/known_hosts
cd <project_folder>
DOCKER_HOST=$(jq -r .docker_host onctl-deploy.json) docker compose up -d --build
```

Have fun!

