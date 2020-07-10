# Description

CronManager is a tool written in Go that is used as a wrapper to start cron job. The application does the following:

1. Executes the command
2. Measures the command execution time
3. Examines the command exit status
The tool uses Node Exporter's TextFile collector to publish the alert to Prometheus.
# Requirements

For the tool to work correctly, you need to have Prometheus **node exporter** installed on the machine with the textfile collector enabled and the directory specified. For example, the node exporter command should be run as follows:

```bash
/opt/prometheus/exporters/node_exporter_current/node_exporter --collector.conntrack --collector.diskstats --collector.entropy --collector.filefd --collector.filesystem --collector.loadavg --collector.mdadm --collector.meminfo --collector.netdev --collector.netstat --collector.stat --collector.time --collector.vmstat --web.listen-address=0.0.0.0:9100 --log.level=info --collector.textfile --collector.textfile.directory=/opt/prometheus/exporters/dist/textfile
```

You can also provide custom collector textfile location using environment variable `COLLECTOR_TEXTFILE_PATH`

```bash
EXPORT COLLECTOR_TEXTFILE_PATH = /custom/path/to/textfile
/opt/prometheus/exporters/node_exporter_current/node_exporter --collector.conntrack --collector.diskstats --collector.entropy --collector.filefd --collector.filesystem --collector.loadavg --collector.mdadm --collector.meminfo --collector.netdev --collector.netstat --collector.stat --collector.time --collector.vmstat --web.listen-address=0.0.0.0:9100 --log.level=info --collector.textfile --collector.textfile.directory=$COLLECTOR_TEXTFILE_PATH
```
# Installation

Build the binary:

```bash
env GOOS=linux go build -o cronmanager
```

Move the program to a directory in your $PATH

```bash
sudo mv cronmanager /usr/local/bin/
```



# Usage

The program can be used as follows:

```bash
cronmanager -c command -n jobname [ -t time in seconds ] [ -l log file ]
```

The `command`is the only mandatory argument. Notice that you cannot a bash shell or any of its shell built-ins as the command. So, the following examples will **<u>not work</u>**:

```bash
cronmanager -c "echo 'hello' > somefile"
cronmanager -c "command1; command2; command3"
```

The expected command is a binary file with optional arguments. For example, the following are **<u>valid</u>** commands for cronmanager:

```bash
cronmanager -c "/usr/bin/php /var/www/webdir/console broadcast:entities:updated -e project -l 20000"
cronmanager -c "/usr/bin/python3 /path/to/python_script.py"
```

# Options

`-c`: The command to execute (required). This parameter is required. Please see the Usage section for caveats.

`-n`: The job name (required). It's a good practice to append `_cron` to the job name for easier distinction when viewing the alerts on Prometheus or Graffana.

`-t`: the time in seconds after which `cronmanager` will alert that the job is taking more than it should. The default is 3600 seconds (1 hour).

`-l`: the log file were you want the cron job to write its output. The default is that any output is trashed.

Notice that if  don't specify `-n` followed by a name, the command will default to "Generic" as the job name.

# Alerting

For the tool to work, the `/opt/prometheus/exporters/dist/textfile/ `path **<u>must exist</u>** on the machine with permissions for the user running cronmanager to write to it. This is the default path where the exporter files exist.

Once cronmanager starts a job, it will wait for the specified seconds (using `-t` or the default 3600 seconds). If the cron is still running, cronmanager writes to a file under the exporters path. The file name consists of the job name followed by the `.prom` extension. For example, if you run the command like this `cronmanager -c "some_command some_arguments" -n "myjob"` the following file will be created: `/opt/prometheus/exporters/dist/textfile/myjob.prom`. The contents of the file are as follows:

```plain
# TYPE cron_job gauge
cron_job{"name=cron1","dimension=failed"} 0
cron_job{"name=cron1","dimension=delayed"} 0
cron_job{"name=cron1","dimension=duration"} 10
```

The numbers change to `1` depending on the issue found with the cron job (delayed/failed or both).