package utils

import (
	"fmt"
	"os"
)

var config = `[mysql]
prompt='mysql [\h] {\u} (\d) > '
#

[client]
user               = %s
password           = %s
port               = %d
socket             = %s/mysql_sandbox%d.sock

[mysqld]
user               = %s
port               = %d
socket             = %s/mysql_sandbox%d.sock
basedir            = %s
datadir            = %s/data
tmpdir             = %s/tmp
lower_case_table_names = 1
pid-file           = %s/data/mysql_sandbox%d.pid
bind-address       = 0.0.0.0
gtid_mode          = on
enforce-gtid-consistency = 1
net_write_timeout=1800
net_read_timeout=1800
max_allowed_packet=16777216
skip_name_resolve=1
log-bin=bin
log-slave-updates
server-id          = %d
skip_slave_start
innodb_buffer_pool_size = 5242880
innodb_read_io_threads=1
innodb_write_io_threads=1
innodb_purge_threads=1
performance_schema=OFF
`

var startScript = `#!/bin/bash
BASEDIR='%s'
export LD_LIBRARY_PATH=$BASEDIR/lib:$BASEDIR/lib/mysql:$LD_LIBRARY_PATH
export DYLD_LIBRARY_PATH=$BASEDIR_/lib:$BASEDIR/lib/mysql:$DYLD_LIBRARY_PATH
MYSQLD_SAFE="$BASEDIR/bin/mysqld_safe"
SBDIR="%s"
PIDFILE="$SBDIR/data/mysql_sandbox%d.pid"

if [ ! -f $MYSQLD_SAFE ]
then
    echo "mysqld_safe not found in $BASEDIR/bin/"
    exit 1
fi
MYSQLD_SAFE_OK=%ssh -n $MYSQLD_SAFE 2>&1%s
if [ "$MYSQLD_SAFE_OK" != "" ]
then
    echo "$MYSQLD_SAFE has errors"
    echo "((( $MYSQLD_SAFE_OK )))"
    exit 1
fi

is_running()
{
    if [ -f $PIDFILE ]
    then
        MYPID=$(cat $PIDFILE)
        ps -p $MYPID | grep $MYPID
    fi
}

TIMEOUT=180
if [ -n "$(is_running)" ]
then
    echo "sandbox server already started (found pid file $PIDFILE)"
else
    if [ -f $PIDFILE ]
    then
        # Server is not running. Removing stale pid-file
        rm -f $PIDFILE
    fi
    CURDIR=%spwd%s
    cd $BASEDIR
    $MYSQLD_SAFE --defaults-file=$SBDIR/my.sandbox.cnf $@ > /dev/null 2>&1 &
    cd $CURDIR
    ATTEMPTS=1
    while [ ! -f $PIDFILE ] 
    do
        ATTEMPTS=$(( $ATTEMPTS + 1 ))
        echo -n "."
        if [ $ATTEMPTS = $TIMEOUT ]
        then
            break
        fi
        sleep 1
    done
fi

if [ -f $PIDFILE ]
then
    echo " sandbox server started"
    #if [ -f $SBDIR/needs_reload ]
    #then
    #    if [ -f $SBDIR/rescue_mysql_dump.sql ]
    #    then
    #        $SBDIR/use mysql < $SBDIR/rescue_mysql_dump.sql
    #    fi
    #    rm $SBDIR/needs_reload
    #fi
else
    echo " sandbox server not started yet"
    exit 1
fi
`

var stopScript = `#!/bin/bash
BASEDIR="%s"
SBDIR="%s"
export LD_LIBRARY_PATH=$BASEDIR/lib:$BASEDIR/lib/mysql:$LD_LIBRARY_PATH
export DYLD_LIBRARY_PATH=$BASEDIR/lib:$BASEDIR/lib/mysql:$DYLD_LIBRARY_PATH
MYSQL_ADMIN="$BASEDIR/bin/mysqladmin"
PIDFILE="$SBDIR/data/mysql_sandbox%d.pid"

is_running()
{
    if [ -f $PIDFILE ]
    then
        MYPID=$(cat $PIDFILE)
        ps -p $MYPID | grep $MYPID
    fi
}

if [ -n "$(is_running)" ]
then
    $MYSQL_ADMIN --defaults-file=$SBDIR/my.sandbox.cnf $MYCLIENT_OPTIONS shutdown
    sleep 1
else
    if [ -f $PIDFILE ]
    then
        rm -f $PIDFILE
    fi
fi

if [ -n "$(is_running)" ]
then
    # use the send_kill script if the server is not responsive
    $SBDIR/send_kill
fi
`

var sendKillScript = `#!/bin/bash
SBDIR="%s"
PIDFILE="$SBDIR/data/mysql_sandbox%d.pid"
TIMEOUT=30

is_running()
{
    if [ -f $PIDFILE ]
    then
        MYPID=$(cat $PIDFILE)
        ps -p $MYPID | grep $MYPID
    fi
}


if [ -n "$(is_running)" ]
then
    MYPID=%scat $PIDFILE%s
    echo "Attempting normal termination --- kill -15 $MYPID"
    kill -15 $MYPID
    # give it a chance to exit peacefully
    ATTEMPTS=1
    while [ -f $PIDFILE ]
    do
        ATTEMPTS=$(( $ATTEMPTS + 1 ))
        if [ $ATTEMPTS = $TIMEOUT ]
        then
            break
        fi
        sleep 1
    done
    if [ -f $PIDFILE ]
    then
        echo "SERVER UNRESPONSIVE --- kill -9 $MYPID"
        kill -9 $MYPID
        rm -f $PIDFILE
    fi
else
    # server not running - removing stale pid-file
    if [ -f $PIDFILE ]
    then
        rm -f $PIDFILE
    fi
fi
`

var useScript = `
#!/bin/bash
export LD_LIBRARY_PATH=%s/lib:%s/lib/mysql:$LD_LIBRARY_PATH
export DYLD_LIBRARY_PATH=%s/lib:%s/lib/mysql:$DYLD_LIBRARY_PATH
SBDIR="%s"
BASEDIR=%s
[ -z "$MYSQL_EDITOR" ] && MYSQL_EDITOR="$BASEDIR/bin/mysql"
HISTDIR=
[ -z "$HISTDIR" ] && HISTDIR=$SBDIR
export MYSQL_HISTFILE="$HISTDIR/.mysql_history"
PIDFILE="$SBDIR/data/mysql_sandbox%d.pid"
if [ -f "$SBINSTR" ]
then
    echo "[%sbasename $0%s] - %sdate "%s"%s - $@" >> $SBINSTR
fi

if [ -f $PIDFILE ]
then
    $MYSQL_EDITOR --defaults-file=$SBDIR/my.sandbox.cnf $MYCLIENT_OPTIONS "$@"
fi
`

var dbscaleServiceScript = `#!/bin/bash
PROG=dbscale
DBSCALE_PATH=%s/dbscale # Need to modify
DBSCALE_PID_FILE=$DBSCALE_PATH/dbscale.pid
DBSCALE_PID=%scat $DBSCALE_PID_FILE 2>/dev/null%s
START_TIMEOUT=60
STOP_TIMEOUT1=10
STOP_TIMEOUT2=5

force=0

check_pid() {
    [ -z $1 ] && return 1
    [ -d "/proc/$1" ] && return 0 || return 1
}

dbscale_start() {
    export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$DBSCALE_PATH/libs
    cd $DBSCALE_PATH

    ./$PROG &
    DBSCALE_PID=$!
    sleep 1

    start_time=%sdate '+%s'%s
    while true; do
        current_time=%sdate '+%s'%s
        if (($current_time - $start_time > $START_TIMEOUT)); then # start time out
            return
        fi
        if [ ! -f "dbscale.pid" ]; then
            continue
        fi
        if [ "X$DBSCALE_PID" == "X"%scat dbscale.pid%s ]; then
            return
        fi
    done
}

dbscale_stop() {
    kill -TERM $DBSCALE_PID >/dev/null 2>&1
    start_time=%sdate '+%s'%s
    while true; do
        current_time=%sdate '+%s'%s
        if (($current_time - $start_time > $STOP_TIMEOUT1)); then
            break
        fi
        if ! check_pid $DBSCALE_PID; then
            return
        fi
    done

    kill -KILL $DBSCALE_PID >/dev/null 2>&1
    sleep $STOP_TIMEOUT2
}

if [ ! -d "$DBSCALE_PATH" -o ! -f "$DBSCALE_PATH/$PROG" ]; then
    echo "The path of dbscale is not correct."
    exit 1
fi

case "$1" in
    start)
        if ! check_pid $DBSCALE_PID; then
            echo -e "Starting DBScale...\c"
            dbscale_start
            check_pid $DBSCALE_PID && echo "done." || echo "fail."
        else
            echo "DBScale has already been running."
        fi
        ;;
    stop)
        if check_pid $DBSCALE_PID; then
            echo -e "Stopping DBScale...\c"
            dbscale_stop
            check_pid $DBSCALE_PID && echo "fail." || echo "done."
        else
            echo "DBScale is not running."
        fi
        ;;
    status)
        if check_pid $DBSCALE_PID; then
            echo "DBScale is running."
            exit 0
        else
            echo "DBScale is not running."
            exit 1
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|status}"
        exit 1
esac
exit 0
`
var dbscaleConfig = `[main]
driver = mysql
log-level = INFO
backlog = 10240
log-file = %s
real-time-queries = 2
admin-user = %s
admin-password = %s
max-replication-delay = 500
default-session-variables = CHARACTER_SET_CLIENT:CHARACTER_SET_RESULTS:CHARACTER_SET_CONNECTION:NET_READ_TIMEOUT:TIME_ZONE:SQL_SAFE_UPDATES:SQL_MODE:AUTOCOMMIT:TX_ISOLATION:SQL_SELECT_LIMIT
support-gtid=1
authenticate-source = auth
is-auth-schema = 1
use-partial-parse = 1
lower-case-table-names = 0
thread-pool-min = 50
thread-pool-max = 80
thread-pool-low = 40
backend-thread-pool-max= 80
handler-thread-pool-max=80
max-fetchnode-ready-rows-size=1000000
auto-inc-lock-mode=0
enable-get-rep-connection = 1
enable-session-swap=0

[driver mysql]
type = MySQLDriver
port = %d
bind-address = 0.0.0.0

[catalog def]
data-source = ds_catalog

[data-source ds_catalog]
type = replication
master = p1m-4-50-18-40
slave = p1s-4-50-18-40
user = %s
password = %s
load-balance-strategy = MASTER-SLAVES

[data-server auth_m]
host = 127.0.0.1
port = %d
user = %s
password = %s

[data-server auth_s]
host = 127.0.0.1
port = %d
user = %s
password = %s

[data-source auth]
type = replication
master = auth_m-4-50-18-40
slave  = auth_s-4-50-18-40
load-balance-strategy = MASTER-SLAVES

[data-server p1m]
host = 127.0.0.1
port = %d
user = %s
password = %s

[data-server p1s]
host = 127.0.0.1
port = %d
user = %s
password = %s

[data-source partition1]
type = replication
master = p1m-10-50-20-40
slave = p1s-10-50-20-40
user = %s
password = %s
load-balance-strategy = MASTER-SLAVES

[data-server p2m]
host = 127.0.0.1
port = %d
user = %s
password = %s

[data-server p2s]
host = 127.0.0.1
port = %d
user = %s
password = %s

[data-source partition2]
type = replication
master = p2m-10-50-20-40
slave = p2s-10-50-20-40
user = %s
password = %s
load-balance-strategy = MASTER-SLAVES

[partition-scheme test]
type=hash
virtual-weight = 1:1
partition = partition1
partition = partition2

[table test.part]
type = partition
pattern=.*
partition-scheme = test
partition-key = id
`

func InitScript4All(installPath string, scriptsDict map[string]string) {
	startAllScript := `#!/bin/bash
SBDIR="%s"
$SBDIR/startallmysql
$SBDIR/dbscale-start.sh
`
	scriptsDict["startall"] = fmt.Sprintf(startAllScript, installPath)
	stopAllScript := `#!/bin/bash
SBDIR="%s"
$SBDIR/dbscale-stop.sh
$SBDIR/stopallmysql
`
	scriptsDict["stopall"] = fmt.Sprintf(stopAllScript, installPath)
}

func InstallScripts4All(installPath string) {
	script4AllDict := make(map[string]string)

	/*** Install startallscript ***/
	InitScript4All(installPath, script4AllDict)
	for scriptName, script := range script4AllDict {
		scriptFilePath := installPath + "/" + scriptName
		scriptFile, err := os.Create(scriptFilePath)
		Check(err)
		_, err = scriptFile.Write([]byte(script))
		Check(err)
		scriptFile.Chmod(0744)
		scriptFile.Close()
	}
}
