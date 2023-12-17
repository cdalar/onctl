# Environment Variables

Environment variables to use for your application

    ``` 
    Usage:
      onctl envs [command]

    Aliases:
      envs, env, environments, environment

    Available Commands:
      add         Add Environment Variable Group
      delete      Delete Environment Variable Group
      describe    Describe Environment Variable Group
      list        List environments variables

    Flags:
      -h, --help   help for envs

    ```

## .env file

env command requests an .env file 

1. Create a simple .env like the one below.

    ``` bash
    DEMO_GREETING="Hello from the environment" 
    DEMO_FAREWELL="Such a sweet sorrow"
    ```

1. Pass it as a paramater to onctl. 

    ```
    ❯ onctl envs add --env-file .env --name test
    ```
    ```
    env-test-env  created.
    ```
    
    !!! warning "Prefix"
        Environment Variable Group names are always prefixes with **env-**. ex. given name _test_ become _env-test_ 

1. List and check envs

    ```
    ❯ onctl env ls
    ```
    ```
    NAME           VALUES   AGE
    env-test       2        9s
    env-website    5        2d3h
    ```

1. List the contents on Environment Variable Group by describe . 
    ```
    ❯ onctl env describe env-test
    ```
    ```
    KEY             VALUE
    DEMO_FAREWELL   Such a sweet sorrow
    DEMO_GREETING   Hello from the environment
    ```


