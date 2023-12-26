# Templates

## initialize virtual machines

- use --init-file (-i in short) to execute an initialization script 
- use --cloud-init-file to set cloud-init script on virtual machines startup.  

## bash

1. use your own script. 

    ```
    onctl up -i scripts/init.sh 
    ```
    to use the file `scripts/init.sh`

1. use an embeded script.

    ```
    onctl up -i docker.sh
    ```
    to use the embeded file. Embeded files can be found under `internal/files/` in github repository.

1. use onctl-templates repo. 

    files on the `onctl-templates` repo can be access directly by using the relative path.

    ```
    onctl up -i wireguard/vpn.sh  # https://templates.onctl.com/wireguard/vpn.sh
    ```

1. use any external source as a HTTP URL.

    any file that is accessiable via URL can be used. 

    ```
    onctl up -i https://gist.githubusercontent.com/cdalar/dabdc001059089f553879a7b535e9b21/raw/02f336857b04eb13bc7ceeec1e66395bd615824b/helloworld.sh
    ```
    to use the embeded file. Embeded files can be found under `internal/files/` in repository.

## cloud-init 

check: [cloud-init docs](https://cloudinit.readthedocs.io/en/latest/){target="_blank"}

To set a cloud-init configuration to your virtual machine. Just add `--cloud-init` flag to your command. 

ex. this command will set the ssh port to 443.
```
onctl up -i wireguard/vpn.sh --cloud-init cloud-init-ssh-443.config
```

## precedence on scripts
1. local file
1. embeded files
1. files on [onctl-templates](https://github.com/cdalar/onctl-templates){target="_blank"} repo
1. as defined on URL (https://example.com)
