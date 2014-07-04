// Copyright 2012, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log/syslog"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	log "github.com/golang/glog"
	"github.com/youtube/vitess/go/flagutil"
	"github.com/youtube/vitess/go/jscfg"
	"github.com/youtube/vitess/go/tb"
	"github.com/youtube/vitess/go/vt/client2"
	hk "github.com/youtube/vitess/go/vt/hook"
	"github.com/youtube/vitess/go/vt/key"
	_ "github.com/youtube/vitess/go/vt/logutil"
	myproto "github.com/youtube/vitess/go/vt/mysqlctl/proto"
	"github.com/youtube/vitess/go/vt/tabletmanager/actionnode"
	"github.com/youtube/vitess/go/vt/tabletmanager/initiator"
	"github.com/youtube/vitess/go/vt/topo"
	"github.com/youtube/vitess/go/vt/wrangler"
)

var (
	noWaitForAction = flag.Bool("no-wait", false, "don't wait for action completion, detach")
	waitTime        = flag.Duration("wait-time", 24*time.Hour, "time to wait on an action")
	lockWaitTimeout = flag.Duration("lock-wait-timeout", 0, "time to wait for a lock before starting an action")
)

type command struct {
	name   string
	method func(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error)
	params string
	help   string // if help is empty, won't list the command
}

type commandGroup struct {
	name     string
	commands []command
}

var commands = []commandGroup{
	commandGroup{
		"Tablets", []command{
			command{"InitTablet", commandInitTablet,
				"[-force] [-parent] [-update] [-db-name-override=<db name>] [-hostname=<hostname>] [-mysql_port=<port>] [-port=<port>] [-vts_port=<port>] [-keyspace=<keyspace>] [-shard=<shard>] [-parent_alias=<parent alias>] <tablet alias> <tablet type>]",
				"Initializes a tablet in the topology.\n" +
					"Valid <tablet type>:\n" +
					"  " + strings.Join(topo.MakeStringTypeList(topo.AllTabletTypes), " ")},
			command{"GetTablet", commandGetTablet,
				"<tablet alias|zk tablet path>",
				"Outputs the json version of Tablet to stdout."},
			command{"UpdateTabletAddrs", commandUpdateTabletAddrs,
				"[-hostname <hostname>] [-ip-addr <ip addr>] [-mysql-port <mysql port>] [-vt-port <vt port>] [-vts-port <vts port>] <tablet alias|zk tablet path> ",
				"Updates the addresses of a tablet."},
			command{"ScrapTablet", commandScrapTablet,
				"[-force] [-skip-rebuild] <tablet alias|zk tablet path>",
				"Scraps a tablet."},
			command{"DeleteTablet", commandDeleteTablet,
				"<tablet alias|zk tablet path> ...",
				"Deletes scrapped tablet(s) from the topology."},
			command{"SetReadOnly", commandSetReadOnly,
				"[<tablet alias|zk tablet path>]",
				"Sets the tablet as ReadOnly."},
			command{"SetReadWrite", commandSetReadWrite,
				"[<tablet alias|zk tablet path>]",
				"Sets the tablet as ReadWrite."},
			command{"SetBlacklistedTables", commandSetBlacklistedTables,
				"[<tablet alias|zk tablet path>] [table1,table2,...]",
				"Sets the list of blacklisted tables for a tablet. Use no tables to clear the list."},
			command{"ChangeSlaveType", commandChangeSlaveType,
				"[-force] [-dry-run] <tablet alias|zk tablet path> <tablet type>",
				"Change the db type for this tablet if possible. This is mostly for arranging replicas - it will not convert a master.\n" +
					"NOTE: This will automatically update the serving graph.\n" +
					"Valid <tablet type>:\n" +
					"  " + strings.Join(topo.MakeStringTypeList(topo.SlaveTabletTypes), " ")},
			command{"Ping", commandPing,
				"<tablet alias|zk tablet path>",
				"Check that the agent is awake and responding - can be blocked by other in-flight operations."},
			command{"RpcPing", commandRpcPing,
				"<tablet alias|zk tablet path>",
				"Check that the agent is awake and responding to RPCs."},
			command{"Query", commandQuery,
				"<cell> <keyspace> <query>",
				"Send a SQL query to a tablet."},
			command{"Sleep", commandSleep,
				"<tablet alias|zk tablet path> <duration>",
				"Block the action queue for the specified duration (mostly for testing)."},
			command{"Snapshot", commandSnapshot,
				"[-force] [-server-mode] [-concurrency=4] <tablet alias|zk tablet path>",
				"Stop mysqld and copy compressed data aside."},
			command{"SnapshotSourceEnd", commandSnapshotSourceEnd,
				"[-slave-start] [-read-write] <tablet alias|zk tablet path> <original tablet type>",
				"Restart Mysql and restore original server type." +
					"Valid <tablet type>:\n" +
					"  " + strings.Join(topo.MakeStringTypeList(topo.AllTabletTypes), " ")},
			command{"Restore", commandRestore,
				"[-fetch-concurrency=3] [-fetch-retry-count=3] [-dont-wait-for-slave-start] <src tablet alias|zk src tablet path> <src manifest file> <dst tablet alias|zk dst tablet path> [<zk new master path>]",
				"Copy the given snaphot from the source tablet and restart replication to the new master path (or uses the <src tablet path> if not specified). If <src manifest file> is 'default', uses the default value.\n" +
					"NOTE: This does not wait for replication to catch up. The destination tablet must be 'idle' to begin with. It will transition to 'spare' once the restore is complete."},
			command{"Clone", commandClone,
				"[-force] [-concurrency=4] [-fetch-concurrency=3] [-fetch-retry-count=3] [-server-mode] <src tablet alias|zk src tablet path> <dst tablet alias|zk dst tablet path> ...",
				"This performs Snapshot and then Restore on all the targets in parallel. The advantage of having separate actions is that one snapshot can be used for many restores, and it's then easier to spread them over time."},
			command{"MultiSnapshot", commandMultiSnapshot,
				"[-force] [-concurrency=8] [-skip-slave-restart] [-maximum-file-size=134217728] -spec='-' [-tables=''] [-exclude_tables=''] <tablet alias|zk tablet path>",
				"Locks mysqld and copy compressed data aside."},
			command{"MultiRestore", commandMultiRestore,
				"[-force] [-concurrency=4] [-fetch-concurrency=4] [-insert-table-concurrency=4] [-fetch-retry-count=3] [-strategy=] <dst tablet alias|destination zk path> <source zk path>...",
				"Restores a snapshot from multiple hosts."},
			command{"ExecuteHook", commandExecuteHook,
				"<tablet alias|zk tablet path> <hook name> [<param1=value1> <param2=value2> ...]",
				"This runs the specified hook on the given tablet."},
			command{"ReadTabletAction", commandReadTabletAction,
				"<action path>)",
				"Displays the action node as json."},
			command{"ExecuteFetch", commandExecuteFetch,
				"[--max_rows=10000] [--want_fields] [--disable_binlogs] <tablet alias|zk tablet path> <sql command>",
				"Runs the given sql command as a DBA on the remote tablet"},
		},
	},
	commandGroup{
		"Shards", []command{
			command{"CreateShard", commandCreateShard,
				"[-force] [-parent] <keyspace/shard|zk shard path>",
				"Creates the given shard"},
			command{"GetShard", commandGetShard,
				"<keyspace/shard|zk shard path>",
				"Outputs the json version of Shard to stdout."},
			command{"RebuildShardGraph", commandRebuildShardGraph,
				"[-cells=a,b] <zk shard path> ... (/zk/global/vt/keyspaces/<keyspace>/shards/<shard>)",
				"Rebuild the replication graph and shard serving data in zk. This may trigger an update to all connected clients."},
			command{"ShardExternallyReparented", commandShardExternallyReparented,
				"<keyspace/shard|zk shard path> <tablet alias|zk tablet path>",
				"Changes metadata to acknowledge a shard master change performed by an external tool."},
			command{"ValidateShard", commandValidateShard,
				"[-ping-tablets] <keyspace/shard|zk shard path>",
				"Validate all nodes reachable from this shard are consistent."},
			command{"ShardReplicationPositions", commandShardReplicationPositions,
				"<keyspace/shard|zk shard path>",
				"Show slave status on all machines in the shard graph."},
			command{"ListShardTablets", commandListShardTablets,
				"<keyspace/shard|zk shard path>)",
				"List all tablets in a given shard."},
			command{"SetShardServedTypes", commandSetShardServedTypes,
				"<keyspace/shard|zk shard path> [<served type1>,<served type2>,...]",
				"Sets a given shard's served types. Does not rebuild any serving graph."},
			command{"ShardMultiRestore", commandShardMultiRestore,
				"[-force] [-concurrency=4] [-fetch-concurrency=4] [-insert-table-concurrency=4] [-fetch-retry-count=3] [-strategy=] [-tables=<table1>,<table2>,...] <keyspace/shard|zk shard path> <source zk path>...",
				"Restore multi-snapshots on all the tablets of a shard."},
			command{"ShardReplicationAdd", commandShardReplicationAdd,
				"<keyspace/shard|zk shard path> <tablet alias|zk tablet path> <parent tablet alias|zk parent tablet path>",
				"HIDDEN Adds an entry to the replication graph in the given cell"},
			command{"ShardReplicationRemove", commandShardReplicationRemove,
				"<keyspace/shard|zk shard path> <tablet alias|zk tablet path>",
				"HIDDEN Removes an entry to the replication graph in the given cell"},
			command{"ShardReplicationFix", commandShardReplicationFix,
				"<cell> <keyspace/shard|zk shard path>",
				"Walks through a ShardReplication object and fixes the first error it encrounters"},
			command{"RemoveShardCell", commandRemoveShardCell,
				"[-force] <keyspace/shard|zk shard path> <cell>",
				"Removes the cell in the shard's Cells list."},
			command{"DeleteShard", commandDeleteShard,
				"<keyspace/shard|zk shard path> ...",
				"Deletes the given shard(s)"},
		},
	},
	commandGroup{
		"Keyspaces", []command{
			command{"CreateKeyspace", commandCreateKeyspace,
				"[-sharding_column_name=name] [-sharding_column_type=type] [-served-from=tablettype1:ks1,tablettype2,ks2,...] [-force] <keyspace name|zk keyspace path>",
				"Creates the given keyspace"},
			command{"GetKeyspace", commandGetKeyspace,
				"<keyspace|zk keyspace path>",
				"Outputs the json version of Keyspace to stdout."},
			command{"SetKeyspaceShardingInfo", commandSetKeyspaceShardingInfo,
				"[-force] <keyspace name|zk keyspace path> [<column name>] [<column type>]",
				"Updates the sharding info for a keyspace"},
			command{"RebuildKeyspaceGraph", commandRebuildKeyspaceGraph,
				"[-cells=a,b] <zk keyspace path> ... (/zk/global/vt/keyspaces/<keyspace>)",
				"Rebuild the serving data for all shards in this keyspace. This may trigger an update to all connected clients."},
			command{"ValidateKeyspace", commandValidateKeyspace,
				"[-ping-tablets] <keyspace name|zk keyspace path>",
				"Validate all nodes reachable from this keyspace are consistent."},
			command{"MigrateServedTypes", commandMigrateServedTypes,
				"[-reverse] <source keyspace/shard|zk source shard path> <served type>",
				"Migrates a serving type from the source shard to the shards it replicates to. Will also rebuild the serving graph."},
			command{"MigrateServedFrom", commandMigrateServedFrom,
				"[-reverse] <destination keyspace/shard|zk destination shard path> <served type>",
				"Makes the destination keyspace/shard serve the given type. Will also rebuild the serving graph."},
		},
	},
	commandGroup{
		"Generic", []command{
			command{"WaitForAction", commandWaitForAction,
				"<action path>",
				"Watch an action node, printing updates, until the action is complete."},
			command{"Resolve", commandResolve,
				"<keyspace>.<shard>.<db type>:<port name>",
				"Read a list of addresses that can answer this query. The port name is usually _mysql or _vtocc."},
			command{"Validate", commandValidate,
				"[-ping-tablets]",
				"Validate all nodes reachable from global replication graph and all tablets in all discoverable cells are consistent."},
			command{"RebuildReplicationGraph", commandRebuildReplicationGraph,
				"<cell1|zk local vt path1>,<cell2|zk local vt path2>... <keyspace1>,<keyspace2>,...",
				"HIDDEN This takes the Thor's hammer approach of recovery and should only be used in emergencies.  cell1,cell2,... are the canonical source of data for the system. This function uses that canonical data to recover the replication graph, at which point further auditing with Validate can reveal any remaining issues."},
			command{"ListAllTablets", commandListAllTablets,
				"<cell name|zk local vt path>",
				"List all tablets in an awk-friendly way."},
			command{"ListTablets", commandListTablets,
				"<tablet alias|zk tablet path> ...",
				"List specified tablets in an awk-friendly way."},
		},
	},
	commandGroup{
		"Schema, Version, Permissions", []command{
			command{"GetSchema", commandGetSchema,
				"[-tables=<table1>,<table2>,...] [-exclude_tables=<table1>,<table2>,...] [-include-views] <tablet alias|zk tablet path>",
				"Display the full schema for a tablet, or just the schema for the provided tables."},
			command{"ReloadSchema", commandReloadSchema,
				"<tablet alias|zk tablet path>",
				"Asks a remote tablet to reload its schema."},
			command{"ValidateSchemaShard", commandValidateSchemaShard,
				"[-exclude_tables=''] [-include-views] <keyspace/shard|zk shard path>",
				"Validate the master schema matches all the slaves."},
			command{"ValidateSchemaKeyspace", commandValidateSchemaKeyspace,
				"[-exclude_tables=''] [-include-views] <keyspace name|zk keyspace path>",
				"Validate the master schema from shard 0 matches all the other tablets in the keyspace."},
			command{"PreflightSchema", commandPreflightSchema,
				"{-sql=<sql> || -sql-file=<filename>} <tablet alias|zk tablet path>",
				"Apply the schema change to a temporary database to gather before and after schema and validate the change. The sql can be inlined or read from a file."},
			command{"ApplySchema", commandApplySchema,
				"[-force] {-sql=<sql> || -sql-file=<filename>} [-skip-preflight] [-stop-replication] <tablet alias|zk tablet path>",
				"Apply the schema change to the specified tablet (allowing replication by default). The sql can be inlined or read from a file. Note this doesn't change any tablet state (doesn't go into 'schema' type)."},
			command{"ApplySchemaShard", commandApplySchemaShard,
				"[-force] {-sql=<sql> || -sql-file=<filename>} [-simple] [-new-parent=<zk tablet path>] <keyspace/shard|zk shard path>",
				"Apply the schema change to the specified shard. If simple is specified, we just apply on the live master. Otherwise we will need to do the shell game. So we will apply the schema change to every single slave. if new_parent is set, we will also reparent (otherwise the master won't be touched at all). Using the force flag will cause a bunch of checks to be ignored, use with care."},
			command{"ApplySchemaKeyspace", commandApplySchemaKeyspace,
				"[-force] {-sql=<sql> || -sql-file=<filename>} [-simple] <keyspace|zk keyspace path>",
				"Apply the schema change to the specified keyspace. If simple is specified, we just apply on the live masters. Otherwise we will need to do the shell game on each shard. So we will apply the schema change to every single slave (running in parallel on all shards, but on one host at a time in a given shard). We will not reparent at the end, so the masters won't be touched at all. Using the force flag will cause a bunch of checks to be ignored, use with care."},

			command{"ValidateVersionShard", commandValidateVersionShard,
				"<keyspace/shard|zk shard path>",
				"Validate the master version matches all the slaves."},
			command{"ValidateVersionKeyspace", commandValidateVersionKeyspace,
				"<keyspace name|zk keyspace path>",
				"Validate the master version from shard 0 matches all the other tablets in the keyspace."},

			command{"GetPermissions", commandGetPermissions,
				"<tablet alias|zk tablet path>",
				"Display the permissions for a tablet."},
			command{"ValidatePermissionsShard", commandValidatePermissionsShard,
				"<keyspace/shard|zk shard path>",
				"Validate the master permissions match all the slaves."},
			command{"ValidatePermissionsKeyspace", commandValidatePermissionsKeyspace,
				"<keyspace name|zk keyspace path>",
				"Validate the master permissions from shard 0 match all the other tablets in the keyspace."},
		},
	},
	commandGroup{
		"Serving Graph", []command{
			command{"GetSrvKeyspace", commandGetSrvKeyspace,
				"<cell> <keyspace>",
				"Outputs the json version of SrvKeyspace to stdout."},
			command{"GetSrvShard", commandGetSrvShard,
				"<cell> <keyspace/shard|zk shard path>",
				"Outputs the json version of SrvShard to stdout."},
			command{"GetEndPoints", commandGetEndPoints,
				"<cell> <keyspace/shard|zk shard path> <tablet type>",
				"Outputs the json version of EndPoints to stdout."},
		},
	},
	commandGroup{
		"Replication Graph", []command{
			command{"GetShardReplication", commandGetShardReplication,
				"<cell> <keyspace/shard|zk shard path>",
				"Outputs the json version of ShardReplication to stdout."},
		},
	},
}

func addCommand(groupName string, c command) {
	for i, group := range commands {
		if group.name == groupName {
			commands[i].commands = append(commands[i].commands, c)
			return
		}
	}
	panic(fmt.Errorf("Trying to add to missing group %v", groupName))
}

var stdin *bufio.Reader

var resolveWildcards = func(wr *wrangler.Wrangler, args []string) ([]string, error) {
	return args, nil
}

func init() {
	// FIXME(msolomon) need to send all of this to stdout
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [global parameters] command [command parameters]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nThe global optional parameters are:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nThe commands are listed below, sorted by group. Use '%s <command> -h' for more help.\n\n", os.Args[0])
		for _, group := range commands {
			fmt.Fprintf(os.Stderr, "%s:\n", group.name)
			for _, cmd := range group.commands {
				if strings.HasPrefix(cmd.help, "HIDDEN") {
					continue
				}
				fmt.Fprintf(os.Stderr, "  %s %s\n", cmd.name, cmd.params)
			}
			fmt.Fprintf(os.Stderr, "\n")
		}
	}
	stdin = bufio.NewReader(os.Stdin)
}

func confirm(prompt string, force bool) bool {
	if force {
		return true
	}
	fmt.Fprintf(os.Stderr, prompt+" [NO/yes] ")
	line, _ := stdin.ReadString('\n')
	return strings.ToLower(strings.TrimSpace(line)) == "yes"
}

func fmtMapAwkable(m map[string]string) string {
	pairs := make([]string, len(m))
	i := 0
	for k, v := range m {
		pairs[i] = fmt.Sprintf("%v: %q", k, v)
		i++
	}
	sort.Strings(pairs)
	return "[" + strings.Join(pairs, " ") + "]"
}

func fmtTabletAwkable(ti *topo.TabletInfo) string {
	keyspace := ti.Keyspace
	shard := ti.Shard
	if keyspace == "" {
		keyspace = "<null>"
	}
	if shard == "" {
		shard = "<null>"
	}
	return fmt.Sprintf("%v %v %v %v %v %v %v", ti.Alias, keyspace, shard, ti.Type, ti.Addr(), ti.MysqlAddr(), fmtMapAwkable(ti.Tags))
}

func fmtAction(action *actionnode.ActionNode) string {
	state := string(action.State)
	// FIXME(msolomon) The default state should really just have the value "queued".
	if action.State == actionnode.ACTION_STATE_QUEUED {
		state = "queued"
	}
	return fmt.Sprintf("%v %v %v %v %v", action.Path, action.Action, state, action.ActionGuid, action.Error)
}

func listTabletsByShard(ts topo.Server, keyspace, shard string) error {
	tabletAliases, err := topo.FindAllTabletAliasesInShard(ts, keyspace, shard)
	if err != nil {
		return err
	}
	return dumpTablets(ts, tabletAliases)
}

func dumpAllTablets(ts topo.Server, zkVtPath string) error {
	tablets, err := wrangler.GetAllTablets(ts, zkVtPath)
	if err != nil {
		return err
	}
	for _, ti := range tablets {
		fmt.Println(fmtTabletAwkable(ti))
	}
	return nil
}

func dumpTablets(ts topo.Server, tabletAliases []topo.TabletAlias) error {
	tabletMap, err := topo.GetTabletMap(ts, tabletAliases)
	if err != nil {
		return err
	}
	for _, tabletAlias := range tabletAliases {
		ti, ok := tabletMap[tabletAlias]
		if !ok {
			log.Warningf("failed to load tablet %v", tabletAlias)
		} else {
			fmt.Println(fmtTabletAwkable(ti))
		}
	}
	return nil
}

func kquery(ts topo.Server, cell, keyspace, query string) error {
	sconn, err := client2.Dial(ts, cell, keyspace, "master", false, 5*time.Second)
	if err != nil {
		return err
	}
	rows, err := sconn.Exec(query, nil)
	if err != nil {
		return err
	}
	cols := rows.Columns()
	fmt.Println(strings.Join(cols, "\t"))

	rowStrs := make([]string, len(cols)+1)
	for row := rows.Next(); row != nil; row = rows.Next() {
		for i, value := range row {
			switch value.(type) {
			case []byte:
				rowStrs[i] = fmt.Sprintf("%q", value)
			default:
				rowStrs[i] = fmt.Sprintf("%v", value)
			}
		}

		fmt.Println(strings.Join(rowStrs, "\t"))
	}
	return nil
}

// getFileParam returns a string containing either flag is not "",
// or the content of the file named flagFile
func getFileParam(flag, flagFile, name string) string {
	if flag != "" {
		if flagFile != "" {
			log.Fatalf("action requires only one of " + name + " or " + name + "-file")
		}
		return flag
	}

	if flagFile == "" {
		log.Fatalf("action requires one of " + name + " or " + name + "-file")
	}
	data, err := ioutil.ReadFile(flagFile)
	if err != nil {
		log.Fatalf("Cannot read file %v: %v", flagFile, err)
	}
	return string(data)
}

func keyspaceParamToKeyspace(param string) string {
	if param[0] == '/' {
		// old zookeeper path, convert to new-style string keyspace
		zkPathParts := strings.Split(param, "/")
		if len(zkPathParts) != 6 || zkPathParts[0] != "" || zkPathParts[1] != "zk" || zkPathParts[2] != "global" || zkPathParts[3] != "vt" || zkPathParts[4] != "keyspaces" {
			log.Fatalf("Invalid keyspace path: %v", param)
		}
		return zkPathParts[5]
	}
	return param
}

// keyspaceParamsToKeyspaces builds a list of keyspaces.
// It supports topology-based wildcards, and plain wildcards.
// For instance:
// /zk/global/vt/keyspaces/one     // using plugin_zktopo
// /zk/global/vt/keyspaces/*       // using plugin_zktopo
// us*                             // using plain matching
// *                               // using plain matching
func keyspaceParamsToKeyspaces(wr *wrangler.Wrangler, params []string) []string {
	result := make([]string, 0, len(params))
	for _, param := range params {
		if param[0] == '/' {
			// this is a topology-specific path
			zkPaths, err := resolveWildcards(wr, params)
			if err != nil {
				log.Fatalf("Failed to resolve wildcard: %v", err)
			}
			for _, zkPath := range zkPaths {
				result = append(result, keyspaceParamToKeyspace(zkPath))
			}
		} else {
			// this is not a path, so assume a keyspace name,
			// possibly with wildcards
			keyspaces, err := topo.ResolveKeyspaceWildcard(wr.TopoServer(), param)
			if err != nil {
				log.Fatalf("Failed to resolve keyspace wildcard %v: %v", param, err)
			}
			result = append(result, keyspaces...)
		}
	}
	return result
}

func shardParamToKeyspaceShard(param string) (string, string) {
	if param[0] == '/' {
		// old zookeeper path, convert to new-style
		zkPathParts := strings.Split(param, "/")
		if len(zkPathParts) != 8 || zkPathParts[0] != "" || zkPathParts[1] != "zk" || zkPathParts[2] != "global" || zkPathParts[3] != "vt" || zkPathParts[4] != "keyspaces" || zkPathParts[6] != "shards" {
			log.Fatalf("Invalid shard path: %v", param)
		}
		return zkPathParts[5], zkPathParts[7]
	}
	zkPathParts := strings.Split(param, "/")
	if len(zkPathParts) != 2 {
		log.Fatalf("Invalid shard path: %v", param)
	}
	return zkPathParts[0], zkPathParts[1]
}

// shardParamsToKeyspaceShards builds a list of keyspace/shard pairs.
// It supports topology-based wildcards, and plain wildcards.
// For instance:
// /zk/global/vt/keyspaces/*/shards/* // using plugin_zktopo
// user/*                             // using plain matching
// */0                                // using plain matching
func shardParamsToKeyspaceShards(wr *wrangler.Wrangler, params []string) []topo.KeyspaceShard {
	result := make([]topo.KeyspaceShard, 0, len(params))
	for _, param := range params {
		if param[0] == '/' {
			// this is a topology-specific path
			zkPaths, err := resolveWildcards(wr, params)
			if err != nil {
				log.Fatalf("Failed to resolve wildcard: %v", err)
			}
			for _, zkPath := range zkPaths {
				keyspace, shard := shardParamToKeyspaceShard(zkPath)
				result = append(result, topo.KeyspaceShard{keyspace, shard})
			}
		} else {
			// this is not a path, so assume a keyspace
			// name / shard name, each possibly with wildcards
			keyspaceShards, err := topo.ResolveShardWildcard(wr.TopoServer(), param)
			if err != nil {
				log.Fatalf("Failed to resolve keyspace/shard wildcard %v: %v", param, err)
			}
			result = append(result, keyspaceShards...)
		}
	}
	return result
}

// tabletParamToTabletAlias takes either an old style ZK tablet path or a
// new style tablet alias as a string, and returns a TabletAlias.
func tabletParamToTabletAlias(param string) topo.TabletAlias {
	if param[0] == '/' {
		// old zookeeper path, convert to new-style string tablet alias
		zkPathParts := strings.Split(param, "/")
		if len(zkPathParts) != 6 || zkPathParts[0] != "" || zkPathParts[1] != "zk" || zkPathParts[3] != "vt" || zkPathParts[4] != "tablets" {
			log.Fatalf("Invalid tablet path: %v", param)
		}
		param = zkPathParts[2] + "-" + zkPathParts[5]
	}
	result, err := topo.ParseTabletAliasString(param)
	if err != nil {
		log.Fatalf("Invalid tablet alias %v: %v", param, err)
	}
	return result
}

// tabletParamsToTabletAliases takes multiple params and converts them
// to tablet aliases.
func tabletParamsToTabletAliases(params []string) []topo.TabletAlias {
	result := make([]topo.TabletAlias, len(params))
	for i, param := range params {
		result[i] = tabletParamToTabletAlias(param)
	}
	return result
}

// tabletRepParamToTabletAlias takes either an old style ZK tablet replication
// path or a new style tablet alias as a string, and returns a
// TabletAlias.
func tabletRepParamToTabletAlias(param string) topo.TabletAlias {
	if param[0] == '/' {
		// old zookeeper replication path, e.g.
		// /zk/global/vt/keyspaces/ruser/shards/10-20/nyc-0000200278
		// convert to new-style string tablet alias
		zkPathParts := strings.Split(param, "/")
		if len(zkPathParts) != 9 || zkPathParts[0] != "" || zkPathParts[1] != "zk" || zkPathParts[2] != "global" || zkPathParts[3] != "vt" || zkPathParts[4] != "keyspaces" || zkPathParts[6] != "shards" {
			log.Fatalf("Invalid tablet replication path: %v", param)
		}
		param = zkPathParts[8]
	}
	result, err := topo.ParseTabletAliasString(param)
	if err != nil {
		log.Fatalf("Invalid tablet alias %v: %v", param, err)
	}
	return result
}

// vtPathToCell takes either an old style ZK vt path /zk/<cell>/vt or
// a new style cell and returns the cell name
func vtPathToCell(param string) string {
	if param[0] == '/' {
		// old zookeeper replication path like /zk/<cell>/vt
		zkPathParts := strings.Split(param, "/")
		if len(zkPathParts) != 4 || zkPathParts[0] != "" || zkPathParts[1] != "zk" || zkPathParts[3] != "vt" {
			log.Fatalf("Invalid vt path: %v", param)
		}
		return zkPathParts[2]
	}
	return param
}

// parseTabletType parses the string tablet type and verifies
// it is an accepted one
func parseTabletType(param string, types []topo.TabletType) topo.TabletType {
	tabletType := topo.TabletType(param)
	if !topo.IsTypeInList(tabletType, types) {
		log.Fatalf("Type %v is not one of: %v", tabletType, strings.Join(topo.MakeStringTypeList(types), " "))
	}
	return tabletType
}

func commandInitTablet(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	var (
		dbNameOverride = subFlags.String("db-name-override", "", "override the name of the db used by vttablet")
		force          = subFlags.Bool("force", false, "will overwrite the node if it already exists")
		parent         = subFlags.Bool("parent", false, "will create the parent shard and keyspace if they don't exist yet")
		update         = subFlags.Bool("update", false, "perform update if a tablet with provided alias exists")
		hostname       = subFlags.String("hostname", "", "server the tablet is running on")
		mysqlPort      = subFlags.Int("mysql_port", 0, "mysql port for the mysql daemon")
		port           = subFlags.Int("port", 0, "main port for the vttablet process")
		vtsPort        = subFlags.Int("vts_port", 0, "encrypted port for the vttablet process")
		keyspace       = subFlags.String("keyspace", "", "keyspace this tablet belongs to")
		shard          = subFlags.String("shard", "", "shard this tablet belongs to")
		parentAlias    = subFlags.String("parent_alias", "", "alias of the mysql parent tablet for this tablet")
		tags           flagutil.StringMapValue
	)
	subFlags.Var(&tags, "tags", "comma separated list of key:value pairs used to tag the tablet")
	subFlags.Parse(args)

	if subFlags.NArg() != 2 {
		log.Fatalf("action InitTablet requires <tablet alias> <tablet type>")
	}
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	tabletType := parseTabletType(subFlags.Arg(1), topo.AllTabletTypes)

	// create tablet record
	tablet := &topo.Tablet{
		Alias:          tabletAlias,
		Hostname:       *hostname,
		Portmap:        make(map[string]int),
		Keyspace:       *keyspace,
		Shard:          *shard,
		Type:           tabletType,
		DbNameOverride: *dbNameOverride,
		Tags:           tags,
	}
	if *port != 0 {
		tablet.Portmap["vt"] = *port
	}
	if *mysqlPort != 0 {
		tablet.Portmap["mysql"] = *mysqlPort
	}
	if *vtsPort != 0 {
		tablet.Portmap["vts"] = *vtsPort
	}
	if *parentAlias != "" {
		tablet.Parent = tabletRepParamToTabletAlias(*parentAlias)
	}

	return "", wr.InitTablet(tablet, *force, *parent, *update)
}

func commandGetTablet(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action GetTablet requires <tablet alias|zk tablet path>")
	}

	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	tabletInfo, err := wr.TopoServer().GetTablet(tabletAlias)
	if err == nil {
		fmt.Println(jscfg.ToJson(tabletInfo))
	}
	return "", err
}

func commandUpdateTabletAddrs(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	hostname := subFlags.String("hostname", "", "fully qualified host name")
	ipAddr := subFlags.String("ip-addr", "", "IP address")
	mysqlPort := subFlags.Int("mysql-port", 0, "mysql port")
	vtPort := subFlags.Int("vt-port", 0, "vt port")
	vtsPort := subFlags.Int("vts-port", 0, "vts port")
	subFlags.Parse(args)

	if subFlags.NArg() != 1 {
		log.Fatalf("action UpdateTabletAddrs requires <tablet alias|zk tablet path>")
	}
	if *ipAddr != "" && net.ParseIP(*ipAddr) == nil {
		log.Fatalf("malformed address: %v", *ipAddr)
	}

	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	return "", wr.TopoServer().UpdateTabletFields(tabletAlias, func(tablet *topo.Tablet) error {
		if *hostname != "" {
			tablet.Hostname = *hostname
		}
		if *ipAddr != "" {
			tablet.IPAddr = *ipAddr
		}
		if *vtPort != 0 || *vtsPort != 0 || *mysqlPort != 0 {
			if tablet.Portmap == nil {
				tablet.Portmap = make(map[string]int)
			}
			if *vtPort != 0 {
				tablet.Portmap["vt"] = *vtPort
			}
			if *vtsPort != 0 {
				tablet.Portmap["vts"] = *vtsPort
			}
			if *mysqlPort != 0 {
				tablet.Portmap["mysql"] = *mysqlPort
			}
		}
		return nil
	})
}

func commandScrapTablet(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	force := subFlags.Bool("force", false, "writes the scrap state in to zk, no questions asked, if a tablet is offline")
	skipRebuild := subFlags.Bool("skip-rebuild", false, "do not rebuild the shard and keyspace graph after scrapping")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ScrapTablet requires <tablet alias|zk tablet path>")
	}

	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	return wr.Scrap(tabletAlias, *force, *skipRebuild)
}

func commandDeleteTablet(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() == 0 {
		log.Fatalf("action DeleteTablet requires at least one <tablet alias|zk tablet path> ...")
	}

	tabletAliases := tabletParamsToTabletAliases(subFlags.Args())
	for _, tabletAlias := range tabletAliases {
		if err := wr.DeleteTablet(tabletAlias); err != nil {
			return "", err
		}
	}
	return "", nil
}

func commandSetReadOnly(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action SetReadOnly requires <tablet alias|zk tablet path>")
	}

	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	return wr.ActionInitiator().SetReadOnly(tabletAlias)
}

func commandSetReadWrite(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action SetReadWrite requires <tablet alias|zk tablet path>")
	}

	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	return wr.ActionInitiator().SetReadWrite(tabletAlias)
}

func commandSetBlacklistedTables(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 && subFlags.NArg() != 2 {
		log.Fatalf("action SetBlacklistedTables requires <tablet alias|zk tablet path> [table1,table2,...]")
	}

	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	var tables []string
	if subFlags.NArg() == 2 {
		tables = strings.Split(subFlags.Arg(1), ",")
	}
	ti, err := wr.TopoServer().GetTablet(tabletAlias)
	if err != nil {
		log.Fatalf("failed reading tablet %v: %v", tabletAlias, err)
	}
	return "", wr.ActionInitiator().SetBlacklistedTables(ti, tables, *waitTime)
}

func commandChangeSlaveType(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	force := subFlags.Bool("force", false, "will change the type in zookeeper, and not run hooks")
	dryRun := subFlags.Bool("dry-run", false, "just list the proposed change")

	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action ChangeSlaveType requires <zk tablet path> <db type>")
	}

	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	newType := parseTabletType(subFlags.Arg(1), topo.AllTabletTypes)
	if *dryRun {
		ti, err := wr.TopoServer().GetTablet(tabletAlias)
		if err != nil {
			log.Fatalf("failed reading tablet %v: %v", tabletAlias, err)
		}
		if !topo.IsTrivialTypeChange(ti.Type, newType) || !topo.IsValidTypeChange(ti.Type, newType) {
			log.Fatalf("invalid type transition %v: %v -> %v", tabletAlias, ti.Type, newType)
		}
		fmt.Printf("- %v\n", fmtTabletAwkable(ti))
		ti.Type = newType
		fmt.Printf("+ %v\n", fmtTabletAwkable(ti))
		return "", nil
	}
	return "", wr.ChangeType(tabletAlias, newType, *force)
}

func commandPing(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action Ping requires <tablet alias|zk tablet path>")
	}
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	return wr.ActionInitiator().Ping(tabletAlias)
}

func commandRpcPing(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action Ping requires <tablet alias|zk tablet path>")
	}
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	return "", wr.ActionInitiator().RpcPing(tabletAlias, *waitTime)
}

func commandQuery(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 3 {
		log.Fatalf("action Query requires 3")
	}
	return "", kquery(wr.TopoServer(), subFlags.Arg(0), subFlags.Arg(1), subFlags.Arg(2))
}

func commandSleep(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action Sleep requires <tablet alias|zk tablet path> <duration>")
	}
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	duration, err := time.ParseDuration(subFlags.Arg(1))
	if err != nil {
		return "", err
	}
	return wr.ActionInitiator().Sleep(tabletAlias, duration)
}

func commandSnapshotSourceEnd(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	slaveStartRequired := subFlags.Bool("slave-start", false, "will restart replication")
	readWrite := subFlags.Bool("read-write", false, "will make the server read-write")
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action SnapshotSourceEnd requires <tablet alias|zk tablet path> <original server type>")
	}

	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	tabletType := parseTabletType(subFlags.Arg(1), topo.AllTabletTypes)
	return "", wr.SnapshotSourceEnd(tabletAlias, *slaveStartRequired, !(*readWrite), tabletType)
}

func commandSnapshot(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	force := subFlags.Bool("force", false, "will force the snapshot for a master, and turn it into a backup")
	serverMode := subFlags.Bool("server-mode", false, "will symlink the data files and leave mysqld stopped")
	concurrency := subFlags.Int("concurrency", 4, "how many compression/checksum jobs to run simultaneously")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action Snapshot requires <tablet alias|zk src tablet path>")
	}

	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	filename, parentAlias, slaveStartRequired, readOnly, originalType, err := wr.Snapshot(tabletAlias, *force, *concurrency, *serverMode)
	if err == nil {
		log.Infof("Manifest: %v", filename)
		log.Infof("ParentAlias: %v", parentAlias)
		if *serverMode {
			log.Infof("SlaveStartRequired: %v", slaveStartRequired)
			log.Infof("ReadOnly: %v", readOnly)
			log.Infof("OriginalType: %v", originalType)
		}
	}
	return "", err
}

func commandRestore(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	dontWaitForSlaveStart := subFlags.Bool("dont-wait-for-slave-start", false, "won't wait for replication to start (useful when restoring from snapshot source that is the replication master)")
	fetchConcurrency := subFlags.Int("fetch-concurrency", 3, "how many files to fetch simultaneously")
	fetchRetryCount := subFlags.Int("fetch-retry-count", 3, "how many times to retry a failed transfer")
	subFlags.Parse(args)
	if subFlags.NArg() != 3 && subFlags.NArg() != 4 {
		log.Fatalf("action Restore requires <src tablet alias|zk src tablet path> <src manifest path> <dst tablet alias|zk dst tablet path> [<zk new master path>]")
	}
	srcTabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	dstTabletAlias := tabletParamToTabletAlias(subFlags.Arg(2))
	parentAlias := srcTabletAlias
	if subFlags.NArg() == 4 {
		parentAlias = tabletParamToTabletAlias(subFlags.Arg(3))
	}
	return "", wr.Restore(srcTabletAlias, subFlags.Arg(1), dstTabletAlias, parentAlias, *fetchConcurrency, *fetchRetryCount, false, *dontWaitForSlaveStart)
}

func commandClone(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	force := subFlags.Bool("force", false, "will force the snapshot for a master, and turn it into a backup")
	concurrency := subFlags.Int("concurrency", 4, "how many compression/checksum jobs to run simultaneously")
	fetchConcurrency := subFlags.Int("fetch-concurrency", 3, "how many files to fetch simultaneously")
	fetchRetryCount := subFlags.Int("fetch-retry-count", 3, "how many times to retry a failed transfer")
	serverMode := subFlags.Bool("server-mode", false, "will keep the snapshot server offline to serve DB files directly")
	subFlags.Parse(args)
	if subFlags.NArg() < 2 {
		log.Fatalf("action Clone requires <src tablet alias|zk src tablet path> <dst tablet alias|zk dst tablet path> ...")
	}

	srcTabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	dstTabletAliases := make([]topo.TabletAlias, subFlags.NArg()-1)
	for i := 1; i < subFlags.NArg(); i++ {
		dstTabletAliases[i-1] = tabletParamToTabletAlias(subFlags.Arg(i))
	}
	return "", wr.Clone(srcTabletAlias, dstTabletAliases, *force, *concurrency, *fetchConcurrency, *fetchRetryCount, *serverMode)
}

func commandMultiRestore(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (status string, err error) {
	fetchRetryCount := subFlags.Int("fetch-retry-count", 3, "how many times to retry a failed transfer")
	concurrency := subFlags.Int("concurrency", 8, "how many concurrent jobs to run simultaneously")
	fetchConcurrency := subFlags.Int("fetch-concurrency", 4, "how many files to fetch simultaneously")
	insertTableConcurrency := subFlags.Int("insert-table-concurrency", 4, "how many tables to load into a single destination table simultaneously")
	strategy := subFlags.String("strategy", "", "which strategy to use for restore, use 'mysqlctl multirestore -help' for more info")
	subFlags.Parse(args)

	if subFlags.NArg() < 2 {
		log.Fatalf("MultiRestore requires <dst tablet alias|destination zk path> <source tablet alias|source zk path>... %v", args)
	}
	destination := tabletParamToTabletAlias(subFlags.Arg(0))
	sources := make([]topo.TabletAlias, subFlags.NArg()-1)
	for i := 1; i < subFlags.NArg(); i++ {
		sources[i-1] = tabletParamToTabletAlias(subFlags.Arg(i))
	}
	err = wr.MultiRestore(destination, sources, *concurrency, *fetchConcurrency, *insertTableConcurrency, *fetchRetryCount, *strategy)
	return
}

func commandMultiSnapshot(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	force := subFlags.Bool("force", false, "will force the snapshot for a master, and turn it into a backup")
	concurrency := subFlags.Int("concurrency", 8, "how many compression jobs to run simultaneously")
	spec := subFlags.String("spec", "-", "shard specification")
	tablesString := subFlags.String("tables", "", "dump only this comma separated list of table regexp")
	excludeTablesString := subFlags.String("exclude_tables", "", "comma separated list of regexps for tables to exclude")
	skipSlaveRestart := subFlags.Bool("skip-slave-restart", false, "after the snapshot is done, do not restart slave replication")
	maximumFilesize := subFlags.Uint64("maximum-file-size", 128*1024*1024, "the maximum size for an uncompressed data file")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action MultiSnapshot requires <src tablet alias|zk src tablet path>")
	}

	shards, err := key.ParseShardingSpec(*spec)
	if err != nil {
		log.Fatalf("multisnapshot failed: %v", err)
	}
	var tables []string
	if *tablesString != "" {
		tables = strings.Split(*tablesString, ",")
	}
	var excludeTables []string
	if *excludeTablesString != "" {
		excludeTables = strings.Split(*excludeTablesString, ",")
	}

	source := tabletParamToTabletAlias(subFlags.Arg(0))
	filenames, parentAlias, err := wr.MultiSnapshot(shards, source, *concurrency, tables, excludeTables, *force, *skipSlaveRestart, *maximumFilesize)

	if err == nil {
		log.Infof("manifest locations: %v", filenames)
		log.Infof("ParentAlias: %v", parentAlias)
	}
	return "", err
}

func commandReadTabletAction(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ReadTabletAction requires <action path>")
	}
	actionPath := subFlags.Arg(0)
	_, data, _, err := wr.TopoServer().ReadTabletActionPath(actionPath)
	if err == nil {
		actionNode, err := actionnode.ActionNodeFromJson(data, actionPath)
		if err == nil {
			fmt.Println(jscfg.ToJson(actionNode))
		}
	}
	return "", err
}

func commandExecuteFetch(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	maxRows := subFlags.Int("max_rows", 10000, "maximum number of rows to allow in reset")
	wantFields := subFlags.Bool("want_fields", false, "also get the field names")
	disableBinlogs := subFlags.Bool("disable_binlogs", false, "disable writing to binlogs during the query")
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action ReadTabletAction requires <tablet alias|zk tablet path> <sql command>")
	}

	alias := tabletParamToTabletAlias(subFlags.Arg(0))
	query := subFlags.Arg(1)
	qr, err := wr.ExecuteFetch(alias, query, *maxRows, *wantFields, *disableBinlogs)
	if err == nil {
		fmt.Println(jscfg.ToJson(qr))
	}
	return "", err
}

func commandExecuteHook(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() < 2 {
		log.Fatalf("action ExecuteHook requires <tablet alias|zk tablet path> <hook name>")
	}

	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	hook := &hk.Hook{Name: subFlags.Arg(1), Parameters: subFlags.Args()[2:]}
	hr, err := wr.ExecuteHook(tabletAlias, hook)
	if err == nil {
		log.Infof(hr.String())
	}
	return "", err
}

func commandCreateShard(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	force := subFlags.Bool("force", false, "will keep going even if the keyspace already exists")
	parent := subFlags.Bool("parent", false, "creates the parent keyspace if it doesn't exist")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action CreateShard requires <keyspace/shard|zk shard path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	if *parent {
		if err := wr.TopoServer().CreateKeyspace(keyspace, &topo.Keyspace{}); err != nil && err != topo.ErrNodeExists {
			return "", err
		}
	}

	err := topo.CreateShard(wr.TopoServer(), keyspace, shard)
	if *force && err == topo.ErrNodeExists {
		log.Infof("shard %v/%v already exists (ignoring error with -force)", keyspace, shard)
		err = nil
	}
	return "", err
}

func commandGetShard(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action GetShard requires <keyspace/shard|zk shard path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	shardInfo, err := wr.TopoServer().GetShard(keyspace, shard)
	if err == nil {
		fmt.Println(jscfg.ToJson(shardInfo))
	}
	return "", err
}

func commandRebuildShardGraph(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	cells := subFlags.String("cells", "", "comma separated list of cells to update")
	subFlags.Parse(args)
	if subFlags.NArg() == 0 {
		log.Fatalf("action RebuildShardGraph requires at least one <zk shard path>")
	}

	var cellArray []string
	if *cells != "" {
		cellArray = strings.Split(*cells, ",")
	}

	keyspaceShards := shardParamsToKeyspaceShards(wr, subFlags.Args())
	for _, ks := range keyspaceShards {
		if err := wr.RebuildShardGraph(ks.Keyspace, ks.Shard, cellArray); err != nil {
			return "", err
		}
	}
	return "", nil
}

func commandShardExternallyReparented(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action ShardExternallyReparented requires <keyspace/shard|zk shard path> <tablet alias|zk tablet path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(1))
	return "", wr.ShardExternallyReparented(keyspace, shard, tabletAlias)
}

func commandValidateShard(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	pingTablets := subFlags.Bool("ping-tablets", true, "ping all tablets during validate")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ValidateShard requires <keyspace/shard|zk shard path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	return "", wr.ValidateShard(keyspace, shard, *pingTablets)
}

func commandShardReplicationPositions(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ShardReplicationPositions requires <keyspace/shard|zk shard path>")
	}
	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	tablets, positions, err := wr.ShardReplicationPositions(keyspace, shard)
	if tablets == nil {
		return "", err
	}

	lines := make([]string, 0, 24)
	for _, rt := range sortReplicatingTablets(tablets, positions) {
		pos := rt.ReplicationPosition
		ti := rt.TabletInfo
		if pos == nil {
			lines = append(lines, fmtTabletAwkable(ti)+" <err> <err> <err>")
		} else {
			lines = append(lines, fmtTabletAwkable(ti)+fmt.Sprintf(" %v:%010d %v:%010d %v", pos.MasterLogFile, pos.MasterLogPosition, pos.MasterLogFileIo, pos.MasterLogPositionIo, pos.SecondsBehindMaster))
		}
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return "", nil
}

func commandListShardTablets(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ListShardTablets requires <keyspace/shard|zk shard path>")
	}
	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	return "", listTabletsByShard(wr.TopoServer(), keyspace, shard)
}

func commandSetShardServedTypes(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 && subFlags.NArg() != 2 {
		log.Fatalf("action SetShardServedTypes requires <keyspace/shard|zk shard path> [<served type1>,<served type2>,...]")
	}
	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	var servedTypes []topo.TabletType
	if subFlags.NArg() == 2 {
		types := strings.Split(subFlags.Arg(1), ",")
		servedTypes = make([]topo.TabletType, 0, len(types))
		for _, t := range types {
			servedTypes = append(servedTypes, parseTabletType(t, []topo.TabletType{topo.TYPE_MASTER, topo.TYPE_REPLICA, topo.TYPE_RDONLY}))
		}
	}

	return "", wr.SetShardServedTypes(keyspace, shard, servedTypes)
}

func commandShardMultiRestore(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (status string, err error) {
	fetchRetryCount := subFlags.Int("fetch-retry-count", 3, "how many times to retry a failed transfer")
	concurrency := subFlags.Int("concurrency", 8, "how many concurrent jobs to run simultaneously")
	fetchConcurrency := subFlags.Int("fetch-concurrency", 4, "how many files to fetch simultaneously")
	insertTableConcurrency := subFlags.Int("insert-table-concurrency", 4, "how many tables to load into a single destination table simultaneously")
	strategy := subFlags.String("strategy", "", "which strategy to use for restore, use 'mysqlctl multirestore -help' for more info")
	tables := subFlags.String("tables", "", "comma separated list of tables to replicate (used for vertical split)")
	subFlags.Parse(args)

	if subFlags.NArg() < 2 {
		log.Fatalf("ShardMultiRestore requires <keyspace/shard|zk shard path> <source tablet alias|source zk path>... %v", args)
	}
	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	sources := make([]topo.TabletAlias, subFlags.NArg()-1)
	for i := 1; i < subFlags.NArg(); i++ {
		sources[i-1] = tabletParamToTabletAlias(subFlags.Arg(i))
	}
	var tableArray []string
	if *tables != "" {
		tableArray = strings.Split(*tables, ",")
	}
	err = wr.ShardMultiRestore(keyspace, shard, sources, tableArray, *concurrency, *fetchConcurrency, *insertTableConcurrency, *fetchRetryCount, *strategy)
	return
}

func commandShardReplicationAdd(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (status string, err error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 3 {
		log.Fatalf("action ShardReplicationAdd requires <keyspace/shard|zk shard path> <tablet alias|zk tablet path> <parent tablet alias|zk parent tablet path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(1))
	parentAlias := tabletParamToTabletAlias(subFlags.Arg(2))
	return "", topo.AddShardReplicationRecord(wr.TopoServer(), keyspace, shard, tabletAlias, parentAlias)
}

func commandShardReplicationRemove(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (status string, err error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action ShardReplicationRemove requires <keyspace/shard|zk shard path> <tablet alias|zk tablet path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(1))
	return "", topo.RemoveShardReplicationRecord(wr.TopoServer(), keyspace, shard, tabletAlias)
}

func commandShardReplicationFix(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (status string, err error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action ShardReplicationRemove requires <cell> <keyspace/shard|zk shard path>")
	}

	cell := subFlags.Arg(0)
	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(1))
	return "", topo.FixShardReplication(wr.TopoServer(), cell, keyspace, shard)
}

func commandRemoveShardCell(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (status string, err error) {
	force := subFlags.Bool("force", false, "will keep going even we can't reach the cell's topology server to check for tablets")
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action RemoveShardCell requires <keyspace/shard|zk shard path> <cell>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	return "", wr.RemoveShardCell(keyspace, shard, subFlags.Arg(1), *force)
}

func commandDeleteShard(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (status string, err error) {
	subFlags.Parse(args)
	if subFlags.NArg() == 0 {
		log.Fatalf("action DeleteShard requires <keyspace/shard|zk shard path> ...")
	}

	keyspaceShards := shardParamsToKeyspaceShards(wr, subFlags.Args())
	for _, ks := range keyspaceShards {
		err := wr.DeleteShard(ks.Keyspace, ks.Shard)
		switch err {
		case nil:
			// keep going
		case topo.ErrNoNode:
			log.Infof("Shard %v/%v doesn't exist, skipping it", ks.Keyspace, ks.Shard)
		default:
			return "", err
		}
	}
	return "", nil
}

func commandCreateKeyspace(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	shardingColumnName := subFlags.String("sharding_column_name", "", "column to use for sharding operations")
	shardingColumnType := subFlags.String("sharding_column_type", "", "type of the column to use for sharding operations")
	force := subFlags.Bool("force", false, "will keep going even if the keyspace already exists")
	var servedFrom flagutil.StringMapValue
	subFlags.Var(&servedFrom, "served-from", "comma separated list of dbtype:keyspace pairs used to serve traffic")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action CreateKeyspace requires <keyspace name|zk keyspace path>")
	}

	keyspace := keyspaceParamToKeyspace(subFlags.Arg(0))
	kit := key.KeyspaceIdType(*shardingColumnType)
	if !key.IsKeyspaceIdTypeInList(kit, key.AllKeyspaceIdTypes) {
		log.Fatalf("invalid sharding_column_type")
	}
	ki := &topo.Keyspace{
		ShardingColumnName: *shardingColumnName,
		ShardingColumnType: kit,
	}
	if len(servedFrom) > 0 {
		ki.ServedFrom = make(map[topo.TabletType]string, len(servedFrom))
		for name, value := range servedFrom {
			tt := topo.TabletType(name)
			if !topo.IsInServingGraph(tt) {
				log.Fatalf("Cannot use tablet type that is not in serving graph: %v", tt)
			}
			ki.ServedFrom[tt] = value
		}
	}
	err := wr.TopoServer().CreateKeyspace(keyspace, ki)
	if *force && err == topo.ErrNodeExists {
		log.Infof("keyspace %v already exists (ignoring error with -force)", keyspace)
		err = nil
	}
	return "", err
}

func commandGetKeyspace(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action GetKeyspace requires <keyspace|zk keyspace path>")
	}

	keyspace := keyspaceParamToKeyspace(subFlags.Arg(0))
	keyspaceInfo, err := wr.TopoServer().GetKeyspace(keyspace)
	if err == nil {
		fmt.Println(jscfg.ToJson(keyspaceInfo))
	}
	return "", err
}

func commandSetKeyspaceShardingInfo(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	force := subFlags.Bool("force", false, "will update the fields even if they're already set, use with care")
	subFlags.Parse(args)
	if subFlags.NArg() > 3 || subFlags.NArg() < 1 {
		log.Fatalf("action SetKeyspaceShardingInfo requires <keyspace name|zk keyspace path> [<column name>] [<column type>]")
	}

	keyspace := keyspaceParamToKeyspace(subFlags.Arg(0))
	columnName := ""
	if subFlags.NArg() >= 2 {
		columnName = subFlags.Arg(1)
	}
	kit := key.KIT_UNSET
	if subFlags.NArg() >= 3 {
		kit = key.KeyspaceIdType(subFlags.Arg(2))
		if !key.IsKeyspaceIdTypeInList(kit, key.AllKeyspaceIdTypes) {
			log.Fatalf("invalid sharding_column_type")
		}
	}

	return "", wr.SetKeyspaceShardingInfo(keyspace, columnName, kit, *force)
}

func commandRebuildKeyspaceGraph(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	cells := subFlags.String("cells", "", "comma separated list of cells to update")
	subFlags.Parse(args)
	if subFlags.NArg() == 0 {
		log.Fatalf("action RebuildKeyspaceGraph requires at least one <zk keyspace path>")
	}

	var cellArray []string
	if *cells != "" {
		cellArray = strings.Split(*cells, ",")
	}

	keyspaces := keyspaceParamsToKeyspaces(wr, subFlags.Args())
	for _, keyspace := range keyspaces {
		if err := wr.RebuildKeyspaceGraph(keyspace, cellArray); err != nil {
			return "", err
		}
	}
	return "", nil
}

func commandValidateKeyspace(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	pingTablets := subFlags.Bool("ping-tablets", false, "ping all tablets during validate")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ValidateKeyspace requires <keyspace name|zk keyspace path>")
	}

	keyspace := keyspaceParamToKeyspace(subFlags.Arg(0))
	return "", wr.ValidateKeyspace(keyspace, *pingTablets)
}

func commandMigrateServedTypes(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	reverse := subFlags.Bool("reverse", false, "move the served type back instead of forward, use in case of trouble")
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action MigrateServedTypes requires <source keyspace/shard|zk source shard path> <served type>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	servedType := parseTabletType(subFlags.Arg(1), []topo.TabletType{topo.TYPE_MASTER, topo.TYPE_REPLICA, topo.TYPE_RDONLY})
	return "", wr.MigrateServedTypes(keyspace, shard, servedType, *reverse)
}

func commandMigrateServedFrom(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	reverse := subFlags.Bool("reverse", false, "move the served from back instead of forward, use in case of trouble")
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action MigrateServedFrom requires <destination keyspace/shard|zk source shard path> <served type>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	servedType := parseTabletType(subFlags.Arg(1), []topo.TabletType{topo.TYPE_MASTER, topo.TYPE_REPLICA, topo.TYPE_RDONLY})
	return "", wr.MigrateServedFrom(keyspace, shard, servedType, *reverse)
}

func commandWaitForAction(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action WaitForAction requires <action path>")
	}
	return subFlags.Arg(0), nil
}

func commandResolve(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action Resolve requires <keyspace>.<shard>.<db type>:<port name>")
	}
	parts := strings.Split(subFlags.Arg(0), ":")
	if len(parts) != 2 {
		log.Fatalf("action Resolve requires <keyspace>.<shard>.<db type>:<port name>")
	}
	namedPort := parts[1]

	parts = strings.Split(parts[0], ".")
	if len(parts) != 3 {
		log.Fatalf("action Resolve requires <keyspace>.<shard>.<db type>:<port name>")
	}

	tabletType := parseTabletType(parts[2], topo.AllTabletTypes)
	addrs, err := topo.LookupVtName(wr.TopoServer(), "local", parts[0], parts[1], tabletType, namedPort)
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		fmt.Printf("%v:%v\n", addr.Target, addr.Port)
	}
	return "", nil
}

func commandValidate(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	pingTablets := subFlags.Bool("ping-tablets", false, "ping all tablets during validate")
	subFlags.Parse(args)

	if subFlags.NArg() != 0 {
		log.Warningf("action Validate doesn't take any parameter any more")
	}
	return "", wr.Validate(*pingTablets)
}

func commandRebuildReplicationGraph(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	// This is sort of a nuclear option.
	subFlags.Parse(args)
	if subFlags.NArg() < 2 {
		log.Fatalf("action RebuildReplicationGraph requires <cell1>,<cell2>,... <keyspace1>,<keyspace2>...")
	}

	cellParams := strings.Split(subFlags.Arg(0), ",")
	resolvedCells, err := resolveWildcards(wr, cellParams)
	if err != nil {
		return "", err
	}
	cells := make([]string, 0, len(cellParams))
	for _, cell := range resolvedCells {
		cells = append(cells, vtPathToCell(cell))
	}

	keyspaceParams := strings.Split(subFlags.Arg(1), ",")
	keyspaces := keyspaceParamsToKeyspaces(wr, keyspaceParams)
	return "", wr.RebuildReplicationGraph(cells, keyspaces)
}

func commandListAllTablets(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ListAllTablets requires <cell name|zk vt path>")
	}

	cell := vtPathToCell(subFlags.Arg(0))
	return "", dumpAllTablets(wr.TopoServer(), cell)
}

func commandListTablets(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() == 0 {
		log.Fatalf("action ListTablets requires <tablet alias|zk tablet path> ...")
	}

	zkPaths, err := resolveWildcards(wr, subFlags.Args())
	if err != nil {
		return "", err
	}
	aliases := make([]topo.TabletAlias, len(zkPaths))
	for i, zkPath := range zkPaths {
		aliases[i] = tabletParamToTabletAlias(zkPath)
	}
	return "", dumpTablets(wr.TopoServer(), aliases)
}

func commandGetSchema(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	tables := subFlags.String("tables", "", "comma separated list of regexps for tables to gather schema information for")
	excludeTables := subFlags.String("exclude_tables", "", "comma separated list of regexps for tables to exclude")
	includeViews := subFlags.Bool("include-views", false, "include views in the output")
	tableNamesOnly := subFlags.Bool("table_names_only", false, "only display the table names that match")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action GetSchema requires <tablet alias|zk tablet path>")
	}
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	var tableArray []string
	if *tables != "" {
		tableArray = strings.Split(*tables, ",")
	}
	var excludeTableArray []string
	if *excludeTables != "" {
		excludeTableArray = strings.Split(*excludeTables, ",")
	}

	sd, err := wr.GetSchema(tabletAlias, tableArray, excludeTableArray, *includeViews)
	if err == nil {
		if *tableNamesOnly {
			for _, td := range sd.TableDefinitions {
				fmt.Println(td.Name)
			}
		} else {
			fmt.Println(jscfg.ToJson(sd))
		}
	}
	return "", err
}

func commandReloadSchema(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ReloadSchema requires <tablet alias|zk tablet path>")
	}
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	return "", wr.ReloadSchema(tabletAlias)
}

func commandValidateSchemaShard(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	excludeTables := subFlags.String("exclude_tables", "", "comma separated list of regexps for tables to exclude")
	includeViews := subFlags.Bool("include-views", false, "include views in the validation")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ValidateSchemaShard requires <keyspace/shard|zk shard path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	var excludeTableArray []string
	if *excludeTables != "" {
		excludeTableArray = strings.Split(*excludeTables, ",")
	}
	return "", wr.ValidateSchemaShard(keyspace, shard, excludeTableArray, *includeViews)
}

func commandValidateSchemaKeyspace(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	excludeTables := subFlags.String("exclude_tables", "", "comma separated list of regexps for tables to exclude")
	includeViews := subFlags.Bool("include-views", false, "include views in the validation")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ValidateSchemaKeyspace requires <keyspace name|zk keyspace path>")
	}

	keyspace := keyspaceParamToKeyspace(subFlags.Arg(0))
	var excludeTableArray []string
	if *excludeTables != "" {
		excludeTableArray = strings.Split(*excludeTables, ",")
	}
	return "", wr.ValidateSchemaKeyspace(keyspace, excludeTableArray, *includeViews)
}

func commandPreflightSchema(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	sql := subFlags.String("sql", "", "sql command")
	sqlFile := subFlags.String("sql-file", "", "file containing the sql commands")
	subFlags.Parse(args)

	if subFlags.NArg() != 1 {
		log.Fatalf("action PreflightSchema requires <tablet alias|zk tablet path>")
	}
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	change := getFileParam(*sql, *sqlFile, "sql")
	scr, err := wr.PreflightSchema(tabletAlias, change)
	if err == nil {
		log.Infof(scr.String())
	}
	return "", err
}

func commandApplySchema(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	force := subFlags.Bool("force", false, "will apply the schema even if preflight schema doesn't match")
	sql := subFlags.String("sql", "", "sql command")
	sqlFile := subFlags.String("sql-file", "", "file containing the sql commands")
	skipPreflight := subFlags.Bool("skip-preflight", false, "do not preflight the schema (use with care)")
	stopReplication := subFlags.Bool("stop-replication", false, "stop replication before applying schema")
	subFlags.Parse(args)

	if subFlags.NArg() != 1 {
		log.Fatalf("action ApplySchema requires <tablet alias|zk tablet path>")
	}
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	change := getFileParam(*sql, *sqlFile, "sql")

	sc := &myproto.SchemaChange{}
	sc.Sql = change
	sc.AllowReplication = !(*stopReplication)

	// do the preflight to get before and after schema
	if !(*skipPreflight) {
		scr, err := wr.PreflightSchema(tabletAlias, sc.Sql)
		if err != nil {
			log.Fatalf("preflight failed: %v", err)
		}
		log.Infof("Preflight: " + scr.String())
		sc.BeforeSchema = scr.BeforeSchema
		sc.AfterSchema = scr.AfterSchema
		sc.Force = *force
	}

	scr, err := wr.ApplySchema(tabletAlias, sc)
	if err == nil {
		log.Infof(scr.String())
	}
	return "", err
}

func commandApplySchemaShard(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	force := subFlags.Bool("force", false, "will apply the schema even if preflight schema doesn't match")
	sql := subFlags.String("sql", "", "sql command")
	sqlFile := subFlags.String("sql-file", "", "file containing the sql commands")
	simple := subFlags.Bool("simple", false, "just apply change on master and let replication do the rest")
	newParent := subFlags.String("new-parent", "", "will reparent to this tablet after the change")
	subFlags.Parse(args)

	if subFlags.NArg() != 1 {
		log.Fatalf("action ApplySchemaShard requires <keyspace/shard|zk shard path>")
	}
	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	change := getFileParam(*sql, *sqlFile, "sql")
	var newParentAlias topo.TabletAlias
	if *newParent != "" {
		newParentAlias = tabletParamToTabletAlias(*newParent)
	}

	if (*simple) && (*newParent != "") {
		log.Fatalf("new_parent for action ApplySchemaShard can only be specified for complex schema upgrades")
	}

	scr, err := wr.ApplySchemaShard(keyspace, shard, change, newParentAlias, *simple, *force)
	if err == nil {
		log.Infof(scr.String())
	}
	return "", err
}

func commandApplySchemaKeyspace(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	force := subFlags.Bool("force", false, "will apply the schema even if preflight schema doesn't match")
	sql := subFlags.String("sql", "", "sql command")
	sqlFile := subFlags.String("sql-file", "", "file containing the sql commands")
	simple := subFlags.Bool("simple", false, "just apply change on master and let replication do the rest")
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ApplySchemaKeyspace requires <keyspace|zk keyspace path>")
	}

	keyspace := keyspaceParamToKeyspace(subFlags.Arg(0))
	change := getFileParam(*sql, *sqlFile, "sql")
	scr, err := wr.ApplySchemaKeyspace(keyspace, change, *simple, *force)
	if err == nil {
		log.Infof(scr.String())
	}
	return "", err
}

func commandValidateVersionShard(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ValidateVersionShard requires <keyspace/shard|zk shard path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	return "", wr.ValidateVersionShard(keyspace, shard)
}

func commandValidateVersionKeyspace(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ValidateVersionKeyspace requires <keyspace name|zk keyspace path>")
	}

	keyspace := keyspaceParamToKeyspace(subFlags.Arg(0))
	return "", wr.ValidateVersionKeyspace(keyspace)
}

func commandGetPermissions(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action GetPermissions requires <tablet alias|zk tablet path>")
	}
	tabletAlias := tabletParamToTabletAlias(subFlags.Arg(0))
	p, err := wr.GetPermissions(tabletAlias)
	if err == nil {
		log.Infof("%v", p.String()) // they can contain '%'
	}
	return "", err
}

func commandValidatePermissionsShard(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ValidatePermissionsShard requires <keyspace/shard|zk shard path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(0))
	return "", wr.ValidatePermissionsShard(keyspace, shard)
}

func commandValidatePermissionsKeyspace(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 1 {
		log.Fatalf("action ValidatePermissionsKeyspace requires <keyspace name|zk keyspace path>")
	}

	keyspace := keyspaceParamToKeyspace(subFlags.Arg(0))
	return "", wr.ValidatePermissionsKeyspace(keyspace)
}

func commandGetSrvKeyspace(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action GetSrvKeyspace requires <cell> <keyspace>")
	}

	srvKeyspace, err := wr.TopoServer().GetSrvKeyspace(subFlags.Arg(0), subFlags.Arg(1))
	if err == nil {
		fmt.Println(jscfg.ToJson(srvKeyspace))
	}
	return "", err
}

func commandGetSrvShard(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action GetSrvShard requires <cell> <keyspace/shard|zk shard path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(1))
	srvShard, err := wr.TopoServer().GetSrvShard(subFlags.Arg(0), keyspace, shard)
	if err == nil {
		fmt.Println(jscfg.ToJson(srvShard))
	}
	return "", err
}

func commandGetEndPoints(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 3 {
		log.Fatalf("action GetEndPoints requires <cell> <keyspace/shard|zk shard path> <tablet type>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(1))
	tabletType := topo.TabletType(subFlags.Arg(2))
	endPoints, err := wr.TopoServer().GetEndPoints(subFlags.Arg(0), keyspace, shard, tabletType)
	if err == nil {
		fmt.Println(jscfg.ToJson(endPoints))
	}
	return "", err
}

func commandGetShardReplication(wr *wrangler.Wrangler, subFlags *flag.FlagSet, args []string) (string, error) {
	subFlags.Parse(args)
	if subFlags.NArg() != 2 {
		log.Fatalf("action GetShardReplication requires <cell> <keyspace/shard|zk shard path>")
	}

	keyspace, shard := shardParamToKeyspaceShard(subFlags.Arg(1))
	shardReplication, err := wr.TopoServer().GetShardReplication(subFlags.Arg(0), keyspace, shard)
	if err == nil {
		fmt.Println(jscfg.ToJson(shardReplication))
	}
	return "", err
}

// signal handling, centralized here
func installSignalHandlers() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		// we got a signal, notify our modules:
		// - initiator will interrupt anything waiting on a tablet action
		// - wrangler will interrupt anything waiting on a shard or
		//   keyspace lock
		initiator.SignalInterrupt()
		wrangler.SignalInterrupt()
	}()
}

func main() {
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Fatalf("panic: %v", tb.Errorf("%v", panicErr))
		}
	}()

	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	action := args[0]
	installSignalHandlers()

	startMsg := fmt.Sprintf("USER=%v SUDO_USER=%v %v", os.Getenv("USER"), os.Getenv("SUDO_USER"), strings.Join(os.Args, " "))

	if syslogger, err := syslog.New(syslog.LOG_INFO, "vtctl "); err == nil {
		syslogger.Info(startMsg)
	} else {
		log.Warningf("cannot connect to syslog: %v", err)
	}

	topoServer := topo.GetServer()
	defer topo.CloseServers()

	wr := wrangler.New(topoServer, *waitTime, *lockWaitTimeout)
	var actionPath string
	var err error

	found := false
	actionLowerCase := strings.ToLower(action)
	for _, group := range commands {
		for _, cmd := range group.commands {
			if strings.ToLower(cmd.name) == actionLowerCase {
				subFlags := flag.NewFlagSet(action, flag.ExitOnError)
				subFlags.Usage = func() {
					fmt.Fprintf(os.Stderr, "Usage: %s %s %s\n\n", os.Args[0], cmd.name, cmd.params)
					fmt.Fprintf(os.Stderr, "%s\n\n", cmd.help)
					subFlags.PrintDefaults()
				}
				actionPath, err = cmd.method(wr, subFlags, args[1:])
				found = true
			}
		}
	}
	if !found {
		fmt.Fprintf(os.Stderr, "Unknown command %#v\n\n", action)
		flag.Usage()
		os.Exit(1)
	}

	if err != nil {
		log.Errorf("action failed: %v %v", action, err)
		//log.Flush()
		os.Exit(255)
	}
	if actionPath != "" {
		if *noWaitForAction {
			fmt.Println(actionPath)
		} else {
			err := wr.ActionInitiator().WaitForCompletion(actionPath, *waitTime)
			if err != nil {
				log.Error(err.Error())
				//log.Flush()
				os.Exit(255)
			} else {
				log.Infof("action completed: %v", actionPath)
			}
		}
	}
}

type rTablet struct {
	*topo.TabletInfo
	*myproto.ReplicationPosition
}

type rTablets []*rTablet

func (rts rTablets) Len() int { return len(rts) }

func (rts rTablets) Swap(i, j int) { rts[i], rts[j] = rts[j], rts[i] }

// Sort for tablet replication.
// master first, then i/o position, then sql position
func (rts rTablets) Less(i, j int) bool {
	// NOTE: Swap order of unpack to reverse sort
	l, r := rts[j], rts[i]
	// l or r ReplicationPosition would be nil if we failed to get
	// the position (put them at the beginning of the list)
	if l.ReplicationPosition == nil {
		return r.ReplicationPosition != nil
	}
	if r.ReplicationPosition == nil {
		return false
	}
	var lTypeMaster, rTypeMaster int
	if l.Type == topo.TYPE_MASTER {
		lTypeMaster = 1
	}
	if r.Type == topo.TYPE_MASTER {
		rTypeMaster = 1
	}
	if lTypeMaster < rTypeMaster {
		return true
	}
	if lTypeMaster == rTypeMaster {
		if l.MapKeyIo() < r.MapKeyIo() {
			return true
		}
		if l.MapKeyIo() == r.MapKeyIo() {
			if l.MapKey() < r.MapKey() {
				return true
			}
		}
	}
	return false
}

func sortReplicatingTablets(tablets []*topo.TabletInfo, positions []*myproto.ReplicationPosition) []*rTablet {
	rtablets := make([]*rTablet, len(tablets))
	for i, pos := range positions {
		rtablets[i] = &rTablet{tablets[i], pos}
	}
	sort.Sort(rTablets(rtablets))
	return rtablets
}
