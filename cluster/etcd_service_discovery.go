// Copyright (c) TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/namespace"
	"github.com/topfreegames/pitaya/config"
	"github.com/topfreegames/pitaya/constants"
	"github.com/topfreegames/pitaya/logger"
	"github.com/topfreegames/pitaya/util"
)

type etcdServiceDiscovery struct {
	cli                 *clientv3.Client
	config              *config.Config
	syncServersInterval time.Duration
	heartbeatTTL        time.Duration
	logHeartbeat        bool
	lastHeartbeatTime   time.Time
	leaseID             clientv3.LeaseID
	serverMapByType     sync.Map
	serverMapByID       sync.Map
	etcdEndpoints       []string
	etcdPrefix          string
	etcdDialTimeout     time.Duration
	running             bool
	server              *Server
	stopChan            chan bool
	lastSyncTime        time.Time
	listeners           []SDListener
}

// NewEtcdServiceDiscovery ctor
func NewEtcdServiceDiscovery(
	config *config.Config,
	server *Server,
	cli ...*clientv3.Client,
) (ServiceDiscovery, error) {
	var client *clientv3.Client
	if len(cli) > 0 {
		client = cli[0]
	}
	sd := &etcdServiceDiscovery{
		config:    config,
		running:   false,
		server:    server,
		listeners: make([]SDListener, 0),
		stopChan:  make(chan bool),
		cli:       client,
	}

	sd.configure()

	return sd, nil
}

func (sd *etcdServiceDiscovery) configure() {
	sd.etcdEndpoints = sd.config.GetStringSlice("pitaya.cluster.sd.etcd.endpoints")
	sd.etcdDialTimeout = sd.config.GetDuration("pitaya.cluster.sd.etcd.dialtimeout")
	sd.etcdPrefix = sd.config.GetString("pitaya.cluster.sd.etcd.prefix")
	sd.heartbeatTTL = sd.config.GetDuration("pitaya.cluster.sd.etcd.heartbeat.ttl")
	sd.logHeartbeat = sd.config.GetBool("pitaya.cluster.sd.etcd.heartbeat.log")
	sd.syncServersInterval = sd.config.GetDuration("pitaya.cluster.sd.etcd.syncservers.interval")
}

func (sd *etcdServiceDiscovery) watchLeaseChan(c <-chan *clientv3.LeaseKeepAliveResponse) {
	for {
		select {
		case <-sd.stopChan:
			return
		case kaRes := <-c:
			if kaRes == nil {
				logger.Log.Warn("sd: error renewing etcd lease, rebootstrapping")
				for {
					err := sd.bootstrap()
					if err != nil {
						logger.Log.Warn("sd: error rebootstrapping lease, will retry in 5 seconds")
						time.Sleep(5 * time.Second)
						continue
					} else {
						return
					}
				}
			} else {
				if sd.logHeartbeat {
					logger.Log.Debugf("sd: etcd lease %x renewed", kaRes.ID)
				}
			}
		}
	}
}

func (sd *etcdServiceDiscovery) bootstrapLease() error {
	// grab lease
	l, err := sd.cli.Grant(context.TODO(), int64(sd.heartbeatTTL.Seconds()))
	if err != nil {
		return err
	}
	sd.leaseID = l.ID
	logger.Log.Debugf("sd: got leaseID: %x", l.ID)
	// this will keep alive forever, when channel c is closed
	// it means we probably have to rebootstrap the lease
	c, err := sd.cli.KeepAlive(context.TODO(), sd.leaseID)
	if err != nil {
		return err
	}
	// need to receive here as per etcd docs
	<-c
	go sd.watchLeaseChan(c)
	return nil
}

func (sd *etcdServiceDiscovery) addServerIntoEtcd(server *Server) error {
	_, err := sd.cli.Put(
		context.TODO(),
		getKey(server.ID, server.Type),
		server.AsJSONString(),
		clientv3.WithLease(sd.leaseID),
	)
	return err
}

func (sd *etcdServiceDiscovery) bootstrapServer(server *Server) error {
	// put key
	err := sd.addServerIntoEtcd(server)
	if err != nil {
		return err
	}

	sd.SyncServers()

	return nil
}

// AddListener adds a listener to etcd service discovery
func (sd *etcdServiceDiscovery) AddListener(listener SDListener) {
	sd.listeners = append(sd.listeners, listener)
}

// AfterInit executes after Init
func (sd *etcdServiceDiscovery) AfterInit() {
}

// BeforeShutdown executes before shutting down
func (sd *etcdServiceDiscovery) BeforeShutdown() {
}

func (sd *etcdServiceDiscovery) notifyListeners(act Action, sv *Server) {
	for _, l := range sd.listeners {
		if act == DEL {
			l.RemoveServer(sv)
		} else if act == ADD {
			l.AddServer(sv)
		}
	}
}

func (sd *etcdServiceDiscovery) deleteServer(serverID string) {
	if actual, ok := sd.serverMapByID.Load(serverID); ok {
		sv := actual.(*Server)
		sd.serverMapByID.Delete(sv.ID)
		if svMap, ok := sd.serverMapByType.Load(sv.Type); ok {
			sm := svMap.(map[string]*Server)
			delete(sm, sv.ID)
		}
		sd.notifyListeners(DEL, sv)
	}
}

func (sd *etcdServiceDiscovery) deleteLocalInvalidServers(actualServers []string) {
	sd.serverMapByID.Range(func(key interface{}, value interface{}) bool {
		k := key.(string)
		if !util.SliceContainsString(actualServers, k) {
			logger.Log.Warnf("deleting invalid local server %s", k)
			sd.deleteServer(k)
		}
		return true
	})
}

func getKey(serverID, serverType string) string {
	return fmt.Sprintf("servers/%s/%s", serverType, serverID)
}

func (sd *etcdServiceDiscovery) getServerFromEtcd(serverType, serverID string) (*Server, error) {
	svKey := getKey(serverID, serverType)
	svEInfo, err := sd.cli.Get(context.TODO(), svKey)
	if err != nil {
		return nil, fmt.Errorf("error getting server: %s from etcd, error: %s", svKey, err.Error())
	}
	if len(svEInfo.Kvs) == 0 {
		return nil, fmt.Errorf("didn't found server: %s in etcd", svKey)
	}
	return parseServer(svEInfo.Kvs[0].Value)
}

// GetServersByType returns a slice with all the servers of a certain type
func (sd *etcdServiceDiscovery) GetServersByType(serverType string) (map[string]*Server, error) {
	if m, ok := sd.serverMapByType.Load(serverType); ok {
		sm := m.(map[string]*Server)
		if len(sm) > 0 {
			return sm, nil
		}
	}
	return nil, constants.ErrNoServersAvailableOfType
}

func (sd *etcdServiceDiscovery) bootstrap() error {
	err := sd.bootstrapLease()
	if err != nil {
		return err
	}

	err = sd.bootstrapServer(sd.server)
	if err != nil {
		return err
	}
	return nil
}

// GetServer returns a server given it's id
func (sd *etcdServiceDiscovery) GetServer(id string) (*Server, error) {
	if sv, ok := sd.serverMapByID.Load(id); ok {
		return sv.(*Server), nil
	}
	return nil, constants.ErrNoServerWithID
}

// Init starts the service discovery client
func (sd *etcdServiceDiscovery) Init() error {
	sd.running = true
	var cli *clientv3.Client
	var err error
	if sd.cli == nil {
		cli, err = clientv3.New(clientv3.Config{
			Endpoints:   sd.etcdEndpoints,
			DialTimeout: sd.etcdDialTimeout,
		})
		if err != nil {
			return err
		}
		sd.cli = cli
	}

	// namespaced etcd :)
	sd.cli.KV = namespace.NewKV(sd.cli.KV, sd.etcdPrefix)
	sd.cli.Watcher = namespace.NewWatcher(sd.cli.Watcher, sd.etcdPrefix)
	sd.cli.Lease = namespace.NewLease(sd.cli.Lease, sd.etcdPrefix)

	err = sd.bootstrap()
	if err != nil {
		return err
	}

	// update servers
	syncServersTicker := time.NewTicker(sd.syncServersInterval)
	go func() {
		for sd.running {
			select {
			case <-syncServersTicker.C:
				err := sd.SyncServers()
				if err != nil {
					logger.Log.Errorf("error resyncing servers: %s", err.Error())
				}
			case <-sd.stopChan:
				return
			}
		}
	}()

	go sd.watchEtcdChanges()
	return nil
}

func parseEtcdKey(key string) (string, string, error) {
	splittedServer := strings.Split(key, "/")
	if len(splittedServer) != 3 {
		return "", "", fmt.Errorf("error parsing etcd key %s (server name can't contain /)", key)
	}
	svType := splittedServer[1]
	svID := splittedServer[2]
	return svType, svID, nil
}

func parseServer(value []byte) (*Server, error) {
	var sv *Server
	err := json.Unmarshal(value, &sv)
	if err != nil {
		logger.Log.Warnf("failed to load server %s, error: %s", sv, err.Error())
	}
	return sv, nil
}

func (sd *etcdServiceDiscovery) printServers() {
	sd.serverMapByType.Range(func(k, v interface{}) bool {
		logger.Log.Debugf("type: %s, servers: %s", k, v)
		return true
	})

}

// SyncServers gets all servers from etcd
func (sd *etcdServiceDiscovery) SyncServers() error {
	keys, err := sd.cli.Get(
		context.TODO(),
		"servers/",
		clientv3.WithPrefix(),
		clientv3.WithKeysOnly(),
	)
	if err != nil {
		return err
	}

	// delete invalid servers (local ones that are not in etcd)
	allIds := make([]string, 0)

	// filter servers I need to grab info
	for _, kv := range keys.Kvs {
		svType, svID, err := parseEtcdKey(string(kv.Key))
		if err != nil {
			logger.Log.Warnf("failed to parse etcd key %s, error: %s", kv.Key, err.Error())
		}
		allIds = append(allIds, svID)
		// TODO is this slow? if so we can paralellize
		if _, ok := sd.serverMapByID.Load(svID); !ok {
			logger.Log.Debugf("loading info from missing server: %s/%s", svType, svID)
			sv, err := sd.getServerFromEtcd(svType, svID)
			if err != nil {
				logger.Log.Errorf("error getting server from etcd: %s, error: %s", svID, err.Error())
				continue
			}
			sd.addServer(sv)
		}
	}
	sd.deleteLocalInvalidServers(allIds)

	sd.printServers()
	sd.lastSyncTime = time.Now()
	return nil
}

// Shutdown executes on shutdown and will clean etcd
func (sd *etcdServiceDiscovery) Shutdown() error {
	sd.running = false
	close(sd.stopChan)

	_, err := sd.cli.Revoke(context.TODO(), sd.leaseID)
	if err != nil {
		return err
	}
	return nil
}

func (sd *etcdServiceDiscovery) addServer(sv *Server) {
	if _, loaded := sd.serverMapByID.LoadOrStore(sv.ID, sv); !loaded {
		mapSvByType, ok := sd.serverMapByType.Load(sv.Type)
		if !ok {
			mapSvByType = make(map[string]*Server)
			sd.serverMapByType.Store(sv.Type, mapSvByType)
		}
		mapSvByType.(map[string]*Server)[sv.ID] = sv
		if sv.ID != sd.server.ID {
			sd.notifyListeners(ADD, sv)
		}
	}
}

func (sd *etcdServiceDiscovery) watchEtcdChanges() {
	w := sd.cli.Watch(context.Background(), "servers/", clientv3.WithPrefix())

	go func(chn clientv3.WatchChan) {
		for sd.running {
			select {
			case wResp := <-chn:
				for _, ev := range wResp.Events {
					switch ev.Type {
					case clientv3.EventTypePut:
						var sv *Server
						var err error
						if sv, err = parseServer(ev.Kv.Value); err != nil {
							logger.Log.Error(err)
							continue
						}
						sd.addServer(sv)
						logger.Log.Debugf("server %s added", ev.Kv.Key)
						sd.printServers()
					case clientv3.EventTypeDelete:
						_, svID, err := parseEtcdKey(string(ev.Kv.Key))
						if err != nil {
							logger.Log.Warn("failed to parse key from etcd: %s", ev.Kv.Key)
							continue
						}
						sd.deleteServer(svID)
						logger.Log.Debugf("server %s deleted", svID)
						sd.printServers()
					}
				}
			case <-sd.stopChan:
				return
			}
		}
	}(w)
}
