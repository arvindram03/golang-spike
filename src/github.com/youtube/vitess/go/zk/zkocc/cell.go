// Copyright 2012, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zkocc

import (
	"flag"
	"fmt"
	"sync"
	"time"

	log "github.com/golang/glog"
	"github.com/youtube/vitess/go/stats"
	"github.com/youtube/vitess/go/zk"
	"launchpad.net/gozk/zookeeper"
)

// a zkCell object represents a zookeeper cell, with a cache and a connection
// to the real cell.
var (
	baseTimeout       = flag.Duration("base-timeout", 30*time.Second, "zookeeper base time out")
	connectTimeout    = flag.Duration("connect-timeout", 30*time.Second, "zookeeper connection time out")
	reconnectInterval = flag.Int("reconnect-interval", 3, "how many seconds to wait between reconnect attempts")
	refreshInterval   = flag.Duration("cache-refresh-interval", 1*time.Second, "how many seconds to wait between cache refreshes")
	refreshCount      = flag.Int("cache-refresh-count", 10, "how many entries to refresh at every tick")
)

// Our state. We need this to be independent as we want to decorelate the
// connection from what clients are asking for.
// For instance, if a cell is not used often, and gets disconnected,
// we want to reconnect in the background, independently of the clients.
// Also we want to support a BACKOFF mode for fast client failure
// reporting while protecting the server from high rates of connections.
const (
	// DISCONNECTED: initial state of the cell.
	// connect will only work in that state, and will go to CONNECTING
	CELL_DISCONNECTED = iota

	// CONNECTING: a 'connect' function started the connection process.
	// It will then go to CONNECTED or BACKOFF. Only one connect
	// function will run at a time.
	// requests will be blocked until the state changes (if it goes to
	// CONNECTED, request will then try to get the value, if it goes to
	// CELL_BACKOFF, they will fail)
	CELL_CONNECTING

	// steady state, when all is good and dandy.
	CELL_CONNECTED

	// BACKOFF: we're waiting for a bit before trying to reconnect.
	// a go routine will go to DISCONNECTED and start login soon.
	// we're failing all requests in this state.
	CELL_BACKOFF
)

var stateNames = map[int64]string{
	CELL_DISCONNECTED: "Disconnected",

	CELL_CONNECTING: "Connecting",
	CELL_CONNECTED:  "Connected",
	CELL_BACKOFF:    "BackOff",
}

type zkCell struct {
	// set at creation
	cellName string
	zkAddr   string
	zcache   *ZkCache
	zkrStats *zkrStats

	// connection related variables
	mutex   sync.Mutex // For connection & state only
	zconn   zk.Conn
	state   int64
	ready   *sync.Cond // will be signaled at connection time
	lastErr error      // last connection error
}

func newZkCell(name, zkaddr string, zkrstats *zkrStats) *zkCell {
	result := &zkCell{cellName: name, zkAddr: zkaddr, zcache: newZkCache(), zkrStats: zkrstats}
	result.ready = sync.NewCond(&result.mutex)
	stats.Publish("Zcell"+name, stats.StringFunc(func() string {

		result.mutex.Lock()
		defer result.mutex.Unlock()
		return stateNames[result.state]
	}))
	go result.backgroundRefresher()
	return result
}

// background routine to initiate a connection sequence
// only connect if state == CELL_DISCONNECTED
// will change state to CELL_CONNECTING during the connection process
// will then change to CELL_CONNECTED (and braodcast the cond)
// or to CELL_BACKOFF (and schedule a new reconnection soon)
func (zcell *zkCell) connect() {
	// change our state, we're working on connecting
	zcell.mutex.Lock()
	if zcell.state != CELL_DISCONNECTED {
		// someone else is already connecting
		zcell.mutex.Unlock()
		return
	}
	zcell.state = CELL_CONNECTING
	zcell.mutex.Unlock()

	// now connect
	zconn, session, err := zk.DialZkTimeout(zcell.zkAddr, *baseTimeout, *connectTimeout)
	if err == nil {
		zcell.zconn = zconn
		go zcell.handleSessionEvents(session)
	}

	// and change our state
	zcell.mutex.Lock()
	if zcell.state != CELL_CONNECTING {
		panic(fmt.Errorf("Unexpected state: %v", zcell.state))
	}
	if err == nil {
		log.Infof("zk cell conn: cell %v connected", zcell.cellName)
		zcell.state = CELL_CONNECTED
		zcell.lastErr = nil

	} else {
		log.Infof("zk cell conn: cell %v connection failed: %v", zcell.cellName, err)
		zcell.state = CELL_BACKOFF
		zcell.lastErr = err

		go func() {
			// we're going to try to reconnect at some point
			// FIXME(alainjobart) backoff algorithm?
			<-time.NewTimer(time.Duration(*reconnectInterval) * time.Second).C

			// switch back to DISCONNECTED, and trigger a connect
			zcell.mutex.Lock()
			zcell.state = CELL_DISCONNECTED
			zcell.mutex.Unlock()
			zcell.connect()
		}()
	}

	// we broadcast on the condition to get everybody unstuck,
	// whether we succeeded to connect or not
	zcell.ready.Broadcast()
	zcell.mutex.Unlock()
}

// the state transitions from the library are not that obvious:
// - If the server connection is delayed (as with using pkill -STOP
//   on the process), the client will get a STATE_CONNECTING message,
//   and then most likely after that a STATE_EXPIRED_SESSION event.
//   We lost all of our watches, we need to reset them.
// - If the server connection dies, and cannot be re-established
//   (server was restarted), the client will get a STATE_CONNECTING message,
//   and then a STATE_CONNECTED when the connection is re-established.
//   The watches will still be valid.
// - If the server connection dies, and a new server comes in (different
//   server root), the client will never connect again (it will try though!).
//   So we'll only get a STATE_CONNECTING and nothing else. The watches
//   won't be valid at all any more.
// Given all these cases, the simpler for now is to always consider a
// STATE_CONNECTING message as a cache invalidation, close the connection
// and start over.
// (alainjobart: Note I've never seen a STATE_CLOSED message)
func (zcell *zkCell) handleSessionEvents(session <-chan zookeeper.Event) {
	for event := range session {
		log.Infof("zk cell conn: cell %v received: %v", zcell.cellName, event)
		switch event.State {
		case zookeeper.STATE_EXPIRED_SESSION, zookeeper.STATE_CONNECTING:
			zcell.zconn.Close()
			fallthrough
		case zookeeper.STATE_CLOSED:
			zcell.mutex.Lock()
			zcell.state = CELL_DISCONNECTED
			zcell.zconn = nil
			zcell.zcache.markForRefresh()
			// for a closed connection, no backoff at first retry
			// if connect fails again, then we'll backoff
			go zcell.connect()
			zcell.mutex.Unlock()
			log.Warningf("zk cell conn: session for cell %v ended: %v", zcell.cellName, event)
			return
		default:
			log.Infof("zk conn cache: session for cell %v event: %v", zcell.cellName, event)
		}
	}
}

func (zcell *zkCell) getConnection() (zk.Conn, error) {
	zcell.mutex.Lock()
	defer zcell.mutex.Unlock()

	switch zcell.state {
	case CELL_CONNECTED:
		// we are already connected, just return the connection
		return zcell.zconn, nil
	case CELL_DISCONNECTED:
		// trigger the connection sequence and wait for connection
		go zcell.connect()
		fallthrough
	case CELL_CONNECTING:
		for zcell.state != CELL_CONNECTED && zcell.state != CELL_BACKOFF {
			zcell.ready.Wait()
		}
		if zcell.state == CELL_CONNECTED {
			return zcell.zconn, nil
		}
	}

	// we are in BACKOFF or failed to connect
	return nil, zcell.lastErr
}

// runs in the background and refreshes the cache if we're in connected state
func (zcell *zkCell) backgroundRefresher() {
	ticker := time.NewTicker(*refreshInterval)
	for _ = range ticker.C {
		// grab a valid connection
		zcell.mutex.Lock()
		// not connected, what can we do?
		if zcell.state != CELL_CONNECTED {
			zcell.mutex.Unlock()
			continue
		}
		zconn := zcell.zconn
		zcell.mutex.Unlock()

		// get a few values to refresh, and ask for them
		zcell.zcache.refreshSomeValues(zconn, *refreshCount)
	}
}
