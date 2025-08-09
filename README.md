# rclogfetch

`rclogfetch` downloads Redis Cloud system or session logs and maintains state to ensure that all log entries are retrieved.

The [Redis Cloud REST API](https://redis.io/docs/latest/operate/rc/api/)  uses a paginated request in order to provide log files to the caller. This implies that, in order to get all logs over time, it's important to keep state `rclogfetch` uses a simple state file to store the last offest fetched from the logs and uses it that to get any new log entries. 


## Usage

```
rclogfetch:
      --api-key string      Redis Cloud API Key
      --append              append to the output file (if not standard output)
      --asc                 sort the log in ascending order (default true)
      --count uint32        number of lines to fetch from the log (maximum 1000) (default 1000)
      --csv                 output in CSV format (default is JSON)
      --desc                sort the log in descending order
      --id uint32           id of the last recored received (used to resume fetching logs)
      --output string       output file (default is standard output)
      --secret-key string   Redis Cloud Secret Key
      --session             fetch the sesssion log
      --statefile string    state file to store the last fetched log line id (default ".rc-log-fetch-state.yaml")
      --system              fetch the system log (default true)
```


Output is written to standard output unless a different file is given on the command line. If a file name is given, the file will be truncated unless the `--append` parameter is provided. 

The command line arguments can be read from a configuration file (.rclogfetch.yaml). For security reasons, it's best to place the API KEY and SECRET KEY in environment variables. `rclogfetch` will automatically check the environment for variables named `RCLOGFETCH_API_KEY` and `RCLOGFETCH_SECRET_KEY`.

