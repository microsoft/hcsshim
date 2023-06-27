# Using the test network agent 

## Create a network agent configuration file 
- Create a file named `nodenetsvc.conf` with the following contents:
    ```
    {
        "ttrpc": "\\\\.\\pipe\\ncproxy-ttrpc",
        "grpc": "127.0.0.1:6669",
        "node_net_svc_addr": "127.0.0.1:6668",
        "timeout": 10,
	    "networking_settings": {
		    "hns_settings": {
			    "switch_name": "name-of-your-switch" 	
		    }
	    }
    }
    ```
    See definitions [here](https://github.com/microsoft/hcsshim/blob/main/internal/tools/networkagent/defs.go) for further customizing this configuration. 

## Run the network agent
```
networkagent.exe --config .\nodenetsvc.conf
```
