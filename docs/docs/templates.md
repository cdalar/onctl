# Templates

## *onctl* can run scripts on virtual machines, 
- use --init-file (-i in short) to execute an initialization script 
- use --cloud-init-file to set cloud-init script on virtual machines startup.  

## bash script on startup

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

## cloud-init script on startup
